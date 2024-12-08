package handlers

type Response struct {
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
	Err  string      `json:"err,omitempty"`
}
