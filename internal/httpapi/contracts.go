package httpapi

type errorResponse struct {
	Error string `json:"error"`
}
type listResponse struct {
	Items any `json:"items"`
}
