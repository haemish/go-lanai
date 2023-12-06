package filecerts

type SourceProperties struct {
	MinTLSVersion string `json:"min-version"`
	Directory     string `json:"directory"`
	CACertFile    string `json:"ca-cert-file"`
	CertFile      string `json:"cert-file"`
	KeyFile       string `json:"key-file"`
	KeyPass       string `json:"key-pass"`
}
