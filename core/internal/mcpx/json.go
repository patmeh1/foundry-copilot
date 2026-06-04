package mcpx
package mcpx

import "encoding/json"

// jsonUnmarshal exists so the manager can keep its imports minimal.
func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
