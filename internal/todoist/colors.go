package todoist

// Colors maps Todoist color names to hex, per https://developer.todoist.com/api/v1/#tag/Colors
var Colors = map[string]string{
	"berry_red":   "#B8255F",
	"red":         "#DC4C3E",
	"orange":      "#C77100",
	"yellow":      "#B29104",
	"olive_green": "#949C31",
	"lime_green":  "#65A33A",
	"green":       "#369307",
	"mint_green":  "#42A393",
	"teal":        "#148FAD",
	"sky_blue":    "#319DC0",
	"light_blue":  "#6988A4",
	"blue":        "#4180FF",
	"grape":       "#692EC2",
	"violet":      "#CA3FEE",
	"lavender":    "#A4698C",
	"magenta":     "#E05095",
	"salmon":      "#C9766F",
	"charcoal":    "#808080",
	"grey":        "#999999",
	"taupe":       "#8F7A69",
}

// ColorNames lists the Todoist colors in the order of the Todoist color picker.
var ColorNames = []string{
	"berry_red", "red", "orange", "yellow", "olive_green", "lime_green", "green",
	"mint_green", "teal", "sky_blue", "light_blue", "blue", "grape", "violet",
	"lavender", "magenta", "salmon", "charcoal", "grey", "taupe",
}

// ColorHex returns the hex for a Todoist color name, defaulting to charcoal.
func ColorHex(name string) string {
	if h, ok := Colors[name]; ok {
		return h
	}
	return Colors["charcoal"]
}
