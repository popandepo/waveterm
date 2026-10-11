// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package staterecord

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// What the record says about how a block or tab looks is read with the same rules as the window applies before it draws a value
// (frontend/app/tab/stacktablook.ts: memberColorValue, tabsymbol.ts: parseTabSymbol), so a program that writes block meta cannot put
// markup or style text into a file that another page may show. A value that does not pass is left out, never cut or repaired. One
// difference, on purpose: the window asks the browser (CSS.supports) whether a colour keyword is a colour; here a keyword must be on the
// list of CSS colour names below (plus transparent and currentcolor), so the record can only be stricter than the window.

const (
	maxColorLength          = 64
	maxSymbolGraphemes      = 2
	maxSymbolTextBytes      = 32
	maxSymbolIconNameLength = 40
	zeroWidthJoiner         = 0x200d
)

var (
	hexColorShape      = regexp.MustCompile(`(?i)^#([0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$`)
	functionColorShape = regexp.MustCompile(`(?i)^(rgb|rgba|hsl|hsla|hwb|lab|lch|oklab|oklch)\([0-9 .,%/a-z+-]*\)$`)
	keywordColorShape  = regexp.MustCompile(`(?i)^[a-z]{3,30}$`)
	iconNameShape      = regexp.MustCompile(`^(solid@|regular@|brands@|custom@)?[a-z0-9-]+(\+(spin|beat|fade|fw))*$`)
)

func isHiddenRune(r rune, allowJoiner bool) bool {
	if allowJoiner && r == zeroWidthJoiner {
		return false
	}
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r)
}

// ColorValue returns the colour as it may be drawn, or "" when it may not.
func ColorValue(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || utf8.RuneCountInString(value) > maxColorLength {
		return ""
	}
	for _, r := range value {
		if isHiddenRune(r, false) {
			return ""
		}
	}
	if hexColorShape.MatchString(value) || functionColorShape.MatchString(value) {
		return value
	}
	if keywordColorShape.MatchString(value) && cssColorNames[strings.ToLower(value)] {
		return value
	}
	return ""
}

func hasMisplacedJoiner(text string) bool {
	joiner := string(rune(zeroWidthJoiner))
	if strings.HasPrefix(text, joiner) || strings.HasSuffix(text, joiner) || strings.Contains(text, joiner+joiner) {
		return true
	}
	runes := []rune(text)
	for i, r := range runes {
		if r != zeroWidthJoiner {
			continue
		}
		if i > 0 && unicode.IsSpace(runes[i-1]) {
			return true
		}
		if i+1 < len(runes) && unicode.IsSpace(runes[i+1]) {
			return true
		}
	}
	return false
}

// SymbolValue returns the icon name or the short text (an emoji, one or two characters) as it may be drawn, or "" when it may not.
func SymbolValue(raw string) string {
	for _, r := range raw {
		if isHiddenRune(r, true) {
			return ""
		}
	}
	value := strings.TrimSpace(raw)
	if value == "" || hasMisplacedJoiner(value) {
		return ""
	}
	if iconNameShape.MatchString(value) {
		if len(value) <= maxSymbolIconNameLength {
			return value
		}
		return ""
	}
	if len(value) > maxSymbolTextBytes || uniseg.GraphemeClusterCount(value) > maxSymbolGraphemes {
		return ""
	}
	return value
}

// The CSS colour names (CSS Color Module Level 4), plus transparent and currentcolor.
var cssColorNames = makeNameSet(`aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond blue blueviolet brown burlywood cadetblue
chartreuse chocolate coral cornflowerblue cornsilk crimson cyan darkblue darkcyan darkgoldenrod darkgray darkgreen darkgrey darkkhaki darkmagenta
darkolivegreen darkorange darkorchid darkred darksalmon darkseagreen darkslateblue darkslategray darkslategrey darkturquoise darkviolet deeppink
deepskyblue dimgray dimgrey dodgerblue firebrick floralwhite forestgreen fuchsia gainsboro ghostwhite gold goldenrod gray green greenyellow grey
honeydew hotpink indianred indigo ivory khaki lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral lightcyan lightgoldenrodyellow
lightgray lightgreen lightgrey lightpink lightsalmon lightseagreen lightskyblue lightslategray lightslategrey lightsteelblue lightyellow lime
limegreen linen magenta maroon mediumaquamarine mediumblue mediumorchid mediumpurple mediumseagreen mediumslateblue mediumspringgreen
mediumturquoise mediumvioletred midnightblue mintcream mistyrose moccasin navajowhite navy oldlace olive olivedrab orange orangered orchid
palegoldenrod palegreen paleturquoise palevioletred papayawhip peachpuff peru pink plum powderblue purple rebeccapurple red rosybrown royalblue
saddlebrown salmon sandybrown seagreen seashell sienna silver skyblue slateblue slategray slategrey snow springgreen steelblue tan teal thistle
tomato turquoise violet wheat white whitesmoke yellow yellowgreen transparent currentcolor`)

func makeNameSet(names string) map[string]bool {
	set := map[string]bool{}
	for _, n := range strings.Fields(names) {
		set[n] = true
	}
	return set
}
