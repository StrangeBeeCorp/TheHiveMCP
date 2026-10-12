package dashboard

import (
	_ "embed"
	"fmt"
	"html"
	"html/template"
	"io"
	"math"
	"strings"
)

//go:embed page.html
var pageHTML string

var pageTemplate = template.Must(template.New("page").Funcs(template.FuncMap{"bars": barsSVG, "hbars": hbarsSVG}).Parse(pageHTML))

// Render writes the page as one HTML file: no script or style from elsewhere, so it opens offline.
func Render(w io.Writer, p Page) error {
	err := pageTemplate.Execute(w, p)
	if err != nil {
		return fmt.Errorf("render the dashboard: %w", err)
	}

	return nil
}

// The charts' geometry, in the SVG's own units, about the width of the column a chart sits in:
// its text reads at the page's size.
const (
	chartW     = 440
	chartH     = 180
	padLeft    = 52
	padBottom  = 24
	padTop     = 12
	barGap     = 2
	hbarRow    = 26
	hbarLabelW = 170
	hbarValueW = 44
	labelEvery = 6 // a week label every six bars
	labelChars = 26
	tickGap    = 6 // between a tick's label and the axis
	minBar     = 2 // a bar above 0 stays visible
	tickRise   = 4 // a tick's label centred on its line
)

// barsSVG draws a series over time: one bar a week, its value and week in the hover tip.
func barsSVG(items []Item) template.HTML {
	if len(items) == 0 {
		return ""
	}

	top := 0.0

	for _, it := range items {
		if !math.IsNaN(it.Value) {
			top = math.Max(top, it.Value)
		}
	}

	if top == 0 {
		top = 1
	}

	var b strings.Builder

	plotH := float64(chartH - padBottom - padTop)
	slot := float64(chartW-padLeft) / float64(len(items))
	base := float64(chartH - padBottom)

	fmt.Fprintf(&b, `<svg class="chart" viewBox="0 0 %d %d" role="img" aria-label="%s">`, chartW, chartH, html.EscapeString(fmt.Sprintf("%d weeks, up to %s", len(items), maxText(items, top))))
	fmt.Fprintf(&b, `<line class="grid" x1="%d" x2="%d" y1="%d" y2="%d"/>`, padLeft, chartW, padTop, padTop)
	fmt.Fprintf(&b, `<text class="tick" x="%d" y="%d" text-anchor="end">%s</text>`, padLeft-tickGap, padTop+tickRise, html.EscapeString(maxText(items, top)))
	fmt.Fprintf(&b, `<line class="axis" x1="%d" x2="%d" y1="%.1f" y2="%.1f"/>`, padLeft, chartW, base, base)
	fmt.Fprintf(&b, `<text class="tick" x="%d" y="%.1f" text-anchor="end">0</text>`, padLeft-tickGap, base+tickRise)

	for i, it := range items {
		x := float64(padLeft) + float64(i)*slot
		h := 0.0

		if !math.IsNaN(it.Value) && it.Value > 0 {
			h = math.Max(minBar, plotH*it.Value/top)
		}

		tip := html.EscapeString(it.Tip)
		// The hit target is the whole slot, taller than the bar.
		fmt.Fprintf(&b, `<g class="mark" data-tip="%s"><rect class="hit" x="%.1f" y="%d" width="%.1f" height="%.1f"/>`, tip, x, padTop, slot, base-padTop)

		if h > 0 {
			fmt.Fprintf(&b, `<path class="bar" d="%s"/>`, roundedTop(x+barGap/2, base, slot-barGap, h))
		}

		fmt.Fprintf(&b, `<title>%s</title></g>`, tip)

		if i%labelEvery == 0 {
			fmt.Fprintf(&b, `<text class="tick" x="%.1f" y="%d">%s</text>`, x, chartH-6, html.EscapeString(it.Label)) //nolint:mnd // under the axis
		}
	}

	b.WriteString(`</svg>`)

	return template.HTML(b.String()) // #nosec G203 -- every text is escaped above
}

// maxText is the top tick's label: the largest bar's own text.
func maxText(items []Item, top float64) string {
	for _, it := range items {
		if it.Value == top {
			return it.Text
		}
	}

	return fmt.Sprintf("%.0f", top)
}

// roundedTop is a bar with a 4-unit rounded data end, its base square on the axis.
func roundedTop(x, base, w, h float64) string {
	r := math.Min(4, math.Min(w/2, h)) //nolint:mnd // the data end's radius

	return fmt.Sprintf("M%.1f %.1fV%.1fQ%.1f %.1f %.1f %.1fH%.1fQ%.1f %.1f %.1f %.1fV%.1fZ",
		x, base, base-h+r, x, base-h, x+r, base-h, x+w-r, x+w, base-h, x+w, base-h+r, base)
}

// hbarsSVG draws a breakdown: one labelled bar an item, the largest first, its value beside it.
func hbarsSVG(items []Item) template.HTML {
	if len(items) == 0 {
		return ""
	}

	top := 0.0
	for _, it := range items {
		top = math.Max(top, it.Value)
	}

	if top == 0 {
		top = 1
	}

	var b strings.Builder

	h := hbarRow*len(items) + 4 //nolint:mnd // room for the last row's descenders
	plotW := float64(chartW - hbarLabelW - hbarValueW)

	fmt.Fprintf(&b, `<svg class="chart" viewBox="0 0 %d %d" role="img" aria-label="%d items">`, chartW, h, len(items))

	for i, it := range items {
		y := float64(i * hbarRow)
		w := math.Max(minBar, plotW*it.Value/top)
		tip := html.EscapeString(it.Tip)

		fmt.Fprintf(&b, `<g class="mark" data-tip="%s"><rect class="hit" x="0" y="%.1f" width="%d" height="%d"/>`, tip, y, chartW, hbarRow)
		fmt.Fprintf(&b, `<text class="label" x="%d" y="%.1f" text-anchor="end">%s</text>`, hbarLabelW-10, y+17, html.EscapeString(shorten(it.Label, labelChars))) //nolint:mnd // the row's baseline
		fmt.Fprintf(&b, `<path class="bar" d="%s"/>`, roundedEnd(hbarLabelW, y+5, w, hbarRow-10))                                                                 //nolint:mnd // the bar's inset
		fmt.Fprintf(&b, `<text class="value" x="%.1f" y="%.1f">%s</text>`, float64(hbarLabelW)+w+6, y+17, html.EscapeString(it.Text))                             //nolint:mnd // beside the bar
		fmt.Fprintf(&b, `<title>%s</title></g>`, tip)
	}

	b.WriteString(`</svg>`)

	return template.HTML(b.String()) // #nosec G203 -- every text is escaped above
}

// roundedEnd is a horizontal bar, square on its baseline, rounded at its data end.
func roundedEnd(x, y, w, h float64) string {
	r := math.Min(4, math.Min(h/2, w)) //nolint:mnd // the data end's radius

	return fmt.Sprintf("M%.1f %.1fH%.1fQ%.1f %.1f %.1f %.1fV%.1fQ%.1f %.1f %.1f %.1fH%.1fZ",
		x, y, x+w-r, x+w, y, x+w, y+r, y+h-r, x+w, y+h, x+w-r, y+h, x)
}

func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}

	return string(r[:n-1]) + "…"
}
