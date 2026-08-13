package query

// SystemInfo represents system status information.
type SystemInfo struct {
	Version  string `json:"version"`
	Database string `json:"database"`
	Cache    string `json:"cache"`
	Uptime   string `json:"uptime"`
}
