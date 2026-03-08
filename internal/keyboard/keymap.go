package keyboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultMap maps key names to (row, col) grid positions on the ITE 8295.
var DefaultMap = map[string][2]int{
	// Row 0: Function row
	"esc":    {0, 0},
	"f1":     {0, 2},
	"f2":     {0, 3},
	"f3":     {0, 4},
	"f4":     {0, 5},
	"f5":     {0, 6},
	"f6":     {0, 7},
	"f7":     {0, 8},
	"f8":     {0, 9},
	"f9":     {0, 10},
	"f10":    {0, 11},
	"f11":    {0, 12},
	"f12":    {0, 13},
	"prtsc":  {0, 15},
	"scroll": {0, 16},
	"pause":  {0, 17},

	// Row 1: Number row
	"grave":      {1, 0},
	"1":          {1, 2},
	"2":          {1, 3},
	"3":          {1, 4},
	"4":          {1, 5},
	"5":          {1, 6},
	"6":          {1, 7},
	"7":          {1, 8},
	"8":          {1, 9},
	"9":          {1, 10},
	"0":          {1, 11},
	"minus":      {1, 12},
	"equal":      {1, 13},
	"backspace":  {1, 14},
	"insert":     {1, 15},
	"home":       {1, 16},
	"pageup":     {1, 17},
	"numlock":    {1, 18},
	"num_slash":  {1, 19},

	// Row 2: QWERTY row
	"tab":       {2, 0},
	"q":         {2, 2},
	"w":         {2, 3},
	"e":         {2, 4},
	"r":         {2, 5},
	"t":         {2, 6},
	"y":         {2, 7},
	"u":         {2, 8},
	"i":         {2, 9},
	"o":         {2, 10},
	"p":         {2, 11},
	"lbracket":  {2, 12},
	"rbracket":  {2, 13},
	"backslash": {2, 14},
	"delete":    {2, 15},
	"end":       {2, 16},
	"pagedown":  {2, 17},
	"num_7":     {2, 18},
	"num_8":     {2, 19},

	// Row 3: Home row
	"capslock":    {3, 0},
	"a":           {3, 2},
	"s":           {3, 3},
	"d":           {3, 4},
	"f":           {3, 5},
	"g":           {3, 6},
	"h":           {3, 7},
	"j":           {3, 8},
	"k":           {3, 9},
	"l":           {3, 10},
	"semicolon":   {3, 11},
	"apostrophe":  {3, 12},
	"enter":       {3, 14},
	"num_4":       {3, 18},
	"num_5":       {3, 19},

	// Row 4: Shift row
	"lshift": {4, 0},
	"z":      {4, 3},
	"x":      {4, 4},
	"c":      {4, 5},
	"v":      {4, 6},
	"b":      {4, 7},
	"n":      {4, 8},
	"m":      {4, 9},
	"comma":  {4, 10},
	"period": {4, 11},
	"slash":  {4, 12},
	"rshift": {4, 14},
	"up":     {4, 16},
	"num_1":  {4, 18},
	"num_2":  {4, 19},

	// Row 5: Bottom row
	"lctrl":   {5, 0},
	"lmeta":   {5, 1},
	"lalt":    {5, 2},
	"space":   {5, 6},
	"ralt":    {5, 10},
	"rmeta":   {5, 11},
	"menu":    {5, 12},
	"rctrl":   {5, 14},
	"left":    {5, 15},
	"down":    {5, 16},
	"right":   {5, 17},
	"num_0":   {5, 18},
	"num_dot": {5, 19},
}

// Aliases maps common key name variants to canonical names.
var Aliases = map[string]string{
	"escape":       "esc",
	"printscreen":  "prtsc",
	"print_screen": "prtsc",
	"scrolllock":   "scroll",
	"scroll_lock":  "scroll",
	"backtick":     "grave",
	"tilde":        "grave",
	"-":            "minus",
	"=":            "equal",
	"bksp":         "backspace",
	"bs":           "backspace",
	"ins":          "insert",
	"pgup":         "pageup",
	"pgdn":         "pagedown",
	"del":          "delete",
	"[":            "lbracket",
	"]":            "rbracket",
	"\\":           "backslash",
	"caps":         "capslock",
	";":            "semicolon",
	"'":            "apostrophe",
	"return":       "enter",
	",":            "comma",
	".":            "period",
	"/":            "slash",
	"win":          "lmeta",
	"super":        "lmeta",
	"alt":          "lalt",
	"altgr":        "ralt",
	"ctrl":         "lctrl",
	"shift":        "lshift",
	"fn":           "lmeta",
}

// GetKeyPosition looks up the (row, col) grid position for a key name.
func GetKeyPosition(name string, keymap map[string][2]int) ([2]int, bool) {
	if keymap == nil {
		keymap = DefaultMap
	}
	n := strings.ToLower(strings.TrimSpace(name))
	if alias, ok := Aliases[n]; ok {
		n = alias
	}
	pos, ok := keymap[n]
	return pos, ok
}

func configDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "avellcc")
}

func keymapFile() string {
	return filepath.Join(configDir(), "keymap.json")
}

// LoadKeymap loads a custom keymap from config, falling back to DefaultMap.
func LoadKeymap() map[string][2]int {
	data, err := os.ReadFile(keymapFile())
	if err != nil {
		return copyMap(DefaultMap)
	}
	var raw map[string][2]int
	if err := json.Unmarshal(data, &raw); err != nil {
		return copyMap(DefaultMap)
	}
	return raw
}

// SaveKeymap saves a keymap to the config file.
func SaveKeymap(keymap map[string][2]int) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(keymap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(keymapFile(), data, 0644)
}

// ListKeys returns sorted list of all known key names.
func ListKeys(keymap map[string][2]int) []string {
	if keymap == nil {
		keymap = DefaultMap
	}
	keys := make([]string, 0, len(keymap))
	for k := range keymap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func copyMap(m map[string][2]int) map[string][2]int {
	c := make(map[string][2]int, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
