// Package surface takes a snapshot of what the model reads from this server: the tools/list
// entries, the prompts/list entries, the resource list and the static resource texts, each with
// its token count. The
// snapshot lives in baselines/surface/ (make surface-propose writes it); the surface gate fails
// when the server and the snapshot differ, so a change to the model's interface is always a
// reviewed diff.
package surface

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/bootstrap"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/resources"
)

// Dir is where the snapshot is committed.
const Dir = "baselines/surface"

const (
	// readTimeout bounds one resource read: a dynamic resource that needs TheHive is not read.
	readTimeout = 2 * time.Second
	// charsPerToken is the usual estimate of how much text one token holds.
	charsPerToken = 4
)

// volatile are the resources that need no TheHive but answer something new on every read.
var volatile = map[string]bool{"hive://config/server-time": true}

// Snapshot is the surface at one revision.
type Snapshot struct {
	Tools []Tool `json:"-"`
	// Prompts are listed only when the server advertises them; it advertises none today.
	Prompts   []Tool     `json:"-"`
	Resources []Resource `json:"resources"`
	Templates []Template `json:"templates"`
}

// Tool is one tools/list entry, or one prompts/list entry, as the model receives it.
type Tool struct {
	Name   string
	JSON   json.RawMessage
	Tokens int
}

// Resource is one resources/list entry. A static resource also has the size and hash of its text.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
	Tokens      int    `json:"tokens,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

// Template is one resources/templates/list entry.
type Template struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}

// Tokens estimates what a text costs a model: its characters divided by 4, rounded up, counted on
// the UTF-8 JSON. The open-pr tool_tokens metric counts escaped JSON (— for a dash), so it
// reads higher for the same tools.
func Tokens(text []byte) int {
	return (len([]rune(string(text))) + charsPerToken - 1) / charsPerToken
}

// Take builds the server as cmd/server does and reads its surface through an in-process client.
func Take(ctx context.Context) (Snapshot, error) {
	return TakeFrom(ctx, bootstrap.GetMCPServerAndRegisterTools())
}

// TakeFrom reads the surface of a server through an in-process client.
func TakeFrom(ctx context.Context, srv *server.MCPServer) (Snapshot, error) {
	c, err := client.NewInProcessClient(srv)
	if err != nil {
		return Snapshot{}, fmt.Errorf("in-process client: %w", err)
	}

	defer func() { _ = c.Close() }()

	err = c.Start(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("start the client: %w", err)
	}

	info, err := c.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		ClientInfo:      mcp.Implementation{Name: "surface", Version: "1"},
	}})
	if err != nil {
		return Snapshot{}, fmt.Errorf("initialize: %w", err)
	}

	var snap Snapshot

	// Each kind is read when the server advertises it: listing one it does not offer is an error.
	if info.Capabilities.Tools != nil {
		snap.Tools, err = tools(ctx, c)
		if err != nil {
			return Snapshot{}, err
		}
	}

	if info.Capabilities.Prompts != nil {
		snap.Prompts, err = prompts(ctx, c)
		if err != nil {
			return Snapshot{}, err
		}
	}

	if info.Capabilities.Resources == nil {
		return snap, nil
	}

	snap.Resources, err = resourceList(ctx, c)
	if err != nil {
		return Snapshot{}, err
	}

	snap.Templates, err = templates(ctx, c)

	return snap, err
}

func tools(ctx context.Context, c *client.Client) ([]Tool, error) {
	list, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}

	out := make([]Tool, 0, len(list.Tools))

	for _, t := range list.Tools {
		data, err := compact(t)
		if err != nil {
			return nil, fmt.Errorf("encode tool %s: %w", t.Name, err)
		}

		out = append(out, Tool{Name: t.Name, JSON: data, Tokens: Tokens(data)})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}

func prompts(ctx context.Context, c *client.Client) ([]Tool, error) {
	list, err := c.ListPrompts(ctx, mcp.ListPromptsRequest{})
	if err != nil {
		return nil, fmt.Errorf("prompts/list: %w", err)
	}

	out := make([]Tool, 0, len(list.Prompts))

	for _, p := range list.Prompts {
		data, err := compact(p)
		if err != nil {
			return nil, fmt.Errorf("encode prompt %s: %w", p.Name, err)
		}

		out = append(out, Tool{Name: p.Name, JSON: data, Tokens: Tokens(data)})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}

// resourceList lists the resources and reads the static ones from the registry, past the
// authentication middleware a read through the server needs.
func resourceList(ctx context.Context, c *client.Client) ([]Resource, error) {
	list, err := c.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		return nil, fmt.Errorf("resources/list: %w", err)
	}

	reg := resources.NewResourceRegistry()
	resources.RegisterStaticResources(reg)
	resources.RegisterDynamicResources(reg)

	out := make([]Resource, 0, len(list.Resources))

	for _, r := range list.Resources {
		res := Resource{URI: r.URI, Name: r.Name, Description: r.Description, MIMEType: r.MIMEType}

		// Read twice: a text that changes between reads is not a fixed text, and neither is a
		// clock, whose two reads may fall in the same second.
		first, ok := readStatic(ctx, reg, r.URI)
		second, again := readStatic(ctx, reg, r.URI)

		if ok && again && string(first) == string(second) && !volatile[r.URI] {
			sum := sha256.Sum256(first)
			res.Tokens, res.SHA256 = Tokens(first), hex.EncodeToString(sum[:])
		}

		out = append(out, res)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].URI < out[j].URI })

	return out, nil
}

// readStatic reads a resource whose text needs no TheHive. A resource that does (it errors or
// times out without one) is listed but not read.
func readStatic(ctx context.Context, reg *resources.ResourceRegistry, uri string) ([]byte, bool) {
	_, handler, err := reg.Get(uri)
	if err != nil {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	req := mcp.ReadResourceRequest{}
	req.Params.URI = uri

	contents, err := handler(ctx, req)
	if err != nil {
		return nil, false
	}

	var text []byte

	for _, c := range contents {
		switch t := c.(type) {
		case mcp.TextResourceContents:
			text = append(text, t.Text...)
		case *mcp.TextResourceContents:
			text = append(text, t.Text...)
		}
	}

	return text, len(text) > 0
}

func templates(ctx context.Context, c *client.Client) ([]Template, error) {
	list, err := c.ListResourceTemplates(ctx, mcp.ListResourceTemplatesRequest{})
	if err != nil {
		return nil, fmt.Errorf("resources/templates/list: %w", err)
	}

	out := make([]Template, 0, len(list.ResourceTemplates))

	for _, t := range list.ResourceTemplates {
		uri := ""
		if t.URITemplate != nil {
			uri = t.URITemplate.Raw()
		}

		out = append(out, Template{URITemplate: uri, Name: t.Name, Description: t.Description, MIMEType: t.MIMEType})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].URITemplate < out[j].URITemplate })

	return out, nil
}
