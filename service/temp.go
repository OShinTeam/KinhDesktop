package service

type SystemInfo struct {
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	NumCPU      int    `json:"num_cpu"`
	Hostname    string `json:"hostname"`
	GoVer       string `json:"go_ver"`
	Time        string `json:"time"`
	ProcessName string `json:"process_name"`
}
