package storage

import "encoding/json"

func Encode(v any) ([]byte, error) { return json.Marshal(v) }
func Decode(b []byte, v any) error { return json.Unmarshal(b, v) }
