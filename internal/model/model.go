package model

type RawFood struct {
	ID struct {
		OID string `json:"$oid"`
	} `json:"_id"`
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	Info       map[string]string `json:"info"`
	Nickname   string            `json:"nickname"`
	Type       string            `json:"type"`
	UpdateTime int64             `json:"update_time"`
	ImageURL   string            `json:"imgUrl"`
}

type Food struct {
	ID          int64      `json:"id"`
	SourceOID   string     `json:"source_oid"`
	Name        string     `json:"name"`
	Nickname    string     `json:"nickname"`
	Type        string     `json:"type"`
	URL         string     `json:"url"`
	ImageURL    string     `json:"image_url"`
	UpdateTime  int64      `json:"update_time"`
	SearchText  string     `json:"-"`
	Nutrients   []Nutrient `json:"nutrients"`
	RawDocument string     `json:"-"`
}

type Nutrient struct {
	Name     string   `json:"name"`
	RawValue string   `json:"raw_value"`
	Amount   *float64 `json:"amount,omitempty"`
	Unit     string   `json:"unit,omitempty"`
	Order    int      `json:"order"`
}

type Alias struct {
	FoodID    int64
	Alias     string
	AliasNorm string
	AliasType string
}

type SearchResult struct {
	Food       Food    `json:"food"`
	MatchType  string  `json:"match_type"`
	Score      float64 `json:"score,omitempty"`
	MatchedBy  string  `json:"matched_by,omitempty"`
	Similarity float64 `json:"similarity,omitempty"`
}

type EmbeddingRecord struct {
	FoodID      int64
	Model       string
	Vector      []float64
	SourceText  string
	VectorDim   int
	UpdatedUnix int64
}
