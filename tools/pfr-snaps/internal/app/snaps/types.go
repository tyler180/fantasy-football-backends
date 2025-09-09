package snaps

import "encoding/json"

// Event is the Lambda payload.
type Event struct {
	Mode           string `json:"mode"`             // ingest_snaps_by_game | materialize_snap_trends
	Season         string `json:"season"`           // e.g., "2024"
	TeamChunkTotal *int   `json:"team_chunk_total"` // PFR fallback only
	TeamChunkIndex *int   `json:"team_chunk_index"` // PFR fallback only
	TeamList       string `json:"team_list"`        // CSV ("SEA,TB") - accepts PFR or NFLverse codes
	// You can add fields here later (e.g., keep_all_pos)
}

// Raw is used by Lambda entrypoint to avoid tight coupling to the event type at the edge.
type Raw = json.RawMessage
