package service

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type ColorPicker struct {
	nextColorCount int
	currentHue     float64
}

func NewColorPicker() *ColorPicker {
	return &ColorPicker{
		nextColorCount: 0,
		currentHue:     80.0, // Start around Yellow-Green
	}
}

func hslToHex(h, s, l float64) string {
	c := (1.0 - math.Abs(2.0*l-1.0)) * s
	x := c * (1.0 - math.Abs(math.Mod(h/60.0, 2.0)-1.0))
	m := l - c/2.0

	var rPrime, gPrime, bPrime float64
	if h >= 0 && h < 60 {
		rPrime, gPrime, bPrime = c, x, 0
	} else if h >= 60 && h < 120 {
		rPrime, gPrime, bPrime = x, c, 0
	} else if h >= 120 && h < 180 {
		rPrime, gPrime, bPrime = 0, c, x
	} else if h >= 180 && h < 240 {
		rPrime, gPrime, bPrime = 0, x, c
	} else if h >= 240 && h < 300 {
		rPrime, gPrime, bPrime = x, 0, c
	} else {
		rPrime, gPrime, bPrime = c, 0, x
	}

	r := int(math.Round((rPrime + m) * 255.0))
	g := int(math.Round((gPrime + m) * 255.0))
	b := int(math.Round((bPrime + m) * 255.0))

	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

func (cp *ColorPicker) NextColor() (string, string) {
	// Golden ratio angle in degrees
	goldenRatioConjugate := 137.50776405

	for {
		// Calculate next hue
		cp.currentHue = math.Mod(cp.currentHue+goldenRatioConjugate, 360.0)

		// Skip the magenta/red/pink/orange zone (approx 280 to 40) to avoid conflict colors
		if cp.currentHue > 280 || cp.currentHue < 40 {
			continue // Skip and calculate next
		}
		break
	}

	// Cycle through different saturation/lightness combinations
	// to provide more variance for colors that land near each other in hue
	var s, l float64
	switch cp.nextColorCount % 3 {
	case 0:
		s, l = 0.70, 0.50 // Standard vibrant
	case 1:
		s, l = 0.85, 0.65 // Lighter/Pastel
	case 2:
		s, l = 0.90, 0.35 // Darker, rich
	}

	hexColor := hslToHex(cp.currentHue, s, l)

	patterns := []string{"bg-solid", "bg-stripes", "bg-dots", "bg-horizontal-lines", "bg-vertical-lines"}
	patternIndex := (cp.nextColorCount / 15) % len(patterns)
	pattern := patterns[patternIndex]

	cp.nextColorCount++

	return hexColor, pattern
}

func GetContrastingTextColor(hexColor string) string {
	if hexColor == "" {
		return "#000000"
	}
	hexStr := strings.Replace(hexColor, "#", "", 1)
	val, err := strconv.ParseInt(hexStr, 16, 32)
	if err != nil {
		return "#000000"
	}
	r := (val >> 16) & 0xff
	g := (val >> 8) & 0xff
	b := val & 0xff

	luminance := (float64(r)*299 + float64(g)*587 + float64(b)*114) / 1000

	if luminance > 128 {
		return "#000000"
	}
	return "#FFFFFF"
}
