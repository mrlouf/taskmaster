package protocol

type Request struct {
	Cmd  string `json:"cmd"` // "start", "stop", "status", "restart", "reload", "shutdown"
	Name string `json:"name"`
}

type Response struct {
	Ok  bool   `json:"ok"`
	Msg string `json:"msg"`
}
