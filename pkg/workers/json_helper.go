package workers

import "encoding/json"

func parseJSON(data string, v interface{}) error {
	return json.Unmarshal([]byte(data), v)
}
