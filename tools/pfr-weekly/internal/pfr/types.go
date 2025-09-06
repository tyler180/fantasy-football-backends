package pfr

// PlayerRow is the normalized record we store.
type PlayerRow struct {
	Player string
	Team   string // primary team (by GS desc, then G desc, then alpha)
	Teams  string // all teams seen, comma-separated
	Age    int
	G      int
	GS     int
	Pos    string
}
