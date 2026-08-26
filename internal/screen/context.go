// Package screen defines the data types for screen and frontmost application context.
package screen

// Context captures the state of the user's screen and frontmost application.
type Context struct {
	AppName     string `json:"app_name"`
	BundleID    string `json:"bundle_id"`
	WindowTitle string `json:"window_title"`
	PID         int32  `json:"pid"`
	Bounds      Rect   `json:"bounds"`
	FramePath   string `json:"frame_path,omitempty"`
	ImageBase64 string `json:"image_base64,omitempty"`
}

// Rect defines a 2D bounding rectangle in screen points.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
