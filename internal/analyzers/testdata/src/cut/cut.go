package cut

type Page struct {
	Items   []int `json:"items"`
	HasMore bool  `json:"hasMore"`
}

type Catalog struct {
	Names     []string `json:"names"`
	Truncated bool     `json:"truncated,omitempty"`
}

type Plain struct {
	Items []int `json:"items"`
}

func pages(items []int, more bool) []any {
	return []any{
		Page{Items: items, HasMore: more},
		Page{items, more},
		Page{Items: items},        // want `this Page leaves HasMore unset`
		&Catalog{Names: nil},      // want `this Catalog leaves Truncated unset`
		Catalog{Truncated: false}, // set, even to false
		Plain{Items: items},
		Page{}, // the zero value of an error return
	}
}
