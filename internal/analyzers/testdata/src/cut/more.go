package cut

// Status names its flag hasMore, but a string is no flag.
type Status struct {
	Items   []int  `json:"items"`
	HasMore string `json:"hasMore"`
}

// Hidden keeps its flag out of the JSON.
type Hidden struct {
	Items     []int `json:"items"`
	Truncated bool  `json:"-"`
}

// Both says in two ways that it was cut.
type Both struct {
	Items     []int `json:"items"`
	HasMore   bool  `json:"hasMore"`
	Truncated bool  `json:"truncated"`
}

func more(items []int) []any {
	return []any{
		&Page{Items: items},                     // want `this Page leaves HasMore unset`
		[]Page{{Items: items}, {HasMore: true}}, // want `this Page leaves HasMore unset`
		Status{Items: items},
		Hidden{Items: items},
		Both{Items: items, HasMore: true}, // want `this Both leaves Truncated unset`
		Both{items, false, false},
	}
}
