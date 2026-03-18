// Command json2chisel converts IQ SDF editor JSON scenes to Chisel source code.
//
// Usage: json2chisel scene.json > scene.chisel
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"asciishader/pkg/chisel/format"
)

// --- JSON schema types ---

type Scene struct {
	Info        Info        `json:"info"`
	Environment Environment `json:"environment"`
	Lighting    Lighting    `json:"lighting"`
	Materials   []Material  `json:"materials"`
	Model       Model       `json:"model"`
}

type Info struct {
	Name string `json:"name"`
}

type Environment struct {
	Background struct {
		Type string `json:"type"`
		Data struct {
			Color [3]float64 `json:"color"`
		} `json:"data"`
	} `json:"background"`
}

type Lighting struct {
	Lights []Light `json:"lights"`
}

type Light struct {
	Enabled bool   `json:"enabled"`
	Type    string `json:"type"`
	Data    struct {
		ShadowEnable bool       `json:"shadowEnable"`
		Color        [3]float64 `json:"color"`
		Intensity    float64    `json:"intensity"`
		Direction    [3]float64 `json:"direction"`
	} `json:"data"`
}

type Material struct {
	UUID    string  `json:"uuid"`
	Opacity float64 `json:"opacity"`
	PBR     struct {
		Color     [3]float64 `json:"color"`
		Roughness float64    `json:"roughness"`
		Metalness float64    `json:"metalness"`
	} `json:"pbr"`
}

type Model struct {
	Groups []Group `json:"groups"`
}

type Group struct {
	Name     string    `json:"name"`
	Elements []Element `json:"elements"`
}

type Element struct {
	Name     string          `json:"name"`
	RM       bool            `json:"rm"`
	Blend    json.RawMessage `json:"blend"`
	Material string          `json:"material"`
	Location [9]float64      `json:"location"`
	Repeat   json.RawMessage `json:"repeat"`
	Prim     json.RawMessage `json:"prim"`
}

// --- Helpers ---

func f(v float64) string {
	if math.Abs(v) < 1e-8 {
		return "0"
	}
	s := fmt.Sprintf("%.6f", v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "-0" {
		return "0"
	}
	return s
}

func linearToSRGB(c float64) int {
	var s float64
	if c <= 0.0031308 {
		s = c * 12.92
	} else {
		s = 1.055*math.Pow(c, 1.0/2.4) - 0.055
	}
	v := int(s*255 + 0.5)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func rgbToHex(r, g, b float64) string {
	return fmt.Sprintf("#%02x%02x%02x", linearToSRGB(r), linearToSRGB(g), linearToSRGB(b))
}

func nearIdentityQuat(qx, qy, qz, qw float64) bool {
	return math.Abs(qx) < 1e-3 && math.Abs(qy) < 1e-3 &&
		math.Abs(qz) < 1e-3 && math.Abs(qw-1) < 1e-3
}

func nearZero3(x, y, z float64) bool {
	return math.Abs(x) < 1e-6 && math.Abs(y) < 1e-6 && math.Abs(z) < 1e-6
}

// --- Conversion ---

func convertPrim(prim json.RawMessage) string {
	var arr []json.RawMessage
	if err := json.Unmarshal(prim, &arr); err != nil || len(arr) == 0 {
		return ""
	}

	var primType string
	json.Unmarshal(arr[0], &primType)

	// Group reference
	if len(arr) == 1 {
		return ""
	}

	switch primType {
	case "SoftBox":
		return convertSoftBox(arr)
	case "Horseshoe":
		return convertHorseshoe(arr)
	case "Egg":
		return convertEgg(arr)
	case "Triangle":
		return convertTriangle(arr)
	default:
		return fmt.Sprintf("/* unsupported: %s */\nsphere(0.1)", primType)
	}
}

func convertSoftBox(arr []json.RawMessage) string {
	var params [6]float64
	json.Unmarshal(arr[1], &params)

	var op string
	json.Unmarshal(arr[2], &op)

	var opParams [3]float64
	json.Unmarshal(arr[3], &opParams)

	var offset [3]float64
	if len(arr) > 4 {
		json.Unmarshal(arr[4], &offset)
	}

	// SoftBox params: [bx, bz, r1, r2, r3, r4]
	// Dimensions ×2 (full extent), radii as-is
	sb := fmt.Sprintf("softbox(%s, %s, %s, %s, %s, %s)",
		f(params[0]*2), f(params[1]*2),
		f(params[2]), f(params[3]), f(params[4]), f(params[5]))

	if op == "Extrude" {
		depth := opParams[0]
		ct := opParams[1]
		cb := opParams[2]
		ox := offset[0]

		args := f(depth * 2) // full extent
		if ct > 1e-8 || cb > 1e-8 {
			args += ", " + f(ct) + ", " + f(cb)
		}
		if ox > 1e-8 {
			args += ", offset: " + f(ox)
		}
		return sb + ".extrude(" + args + ")"
	} else if op == "Revolve" {
		return sb + ".revolve(" + f(opParams[0]) + ")"
	}
	return sb
}

func convertHorseshoe(arr []json.RawMessage) string {
	var params [6]float64
	json.Unmarshal(arr[1], &params)

	var op string
	json.Unmarshal(arr[2], &op)

	var opParams [3]float64
	json.Unmarshal(arr[3], &opParams)

	hs := fmt.Sprintf("horseshoe(%s, %s, %s, %s)",
		f(params[0]), f(params[1]), f(params[2]), f(params[3]))

	if op == "Extrude" {
		args := f(opParams[0] * 2)
		if opParams[1] > 1e-8 || opParams[2] > 1e-8 {
			args += ", " + f(opParams[1]) + ", " + f(opParams[2])
		}
		return hs + ".extrude(" + args + ")"
	} else if op == "Revolve" {
		return hs + ".revolve(" + f(opParams[0]) + ")"
	}
	return hs
}

func convertEgg(arr []json.RawMessage) string {
	var params [4]float64
	json.Unmarshal(arr[1], &params)

	var op string
	json.Unmarshal(arr[2], &op)

	var opParams [3]float64
	json.Unmarshal(arr[3], &opParams)

	egg := fmt.Sprintf("egg(%s, %s, %s, %s)",
		f(params[0]), f(params[1]), f(params[2]), f(params[3]))

	if op == "Revolve" {
		return egg + ".revolve(" + f(opParams[0]) + ")"
	} else if op == "Extrude" {
		return egg + ".extrude(" + f(opParams[0]*2) + ")"
	}
	return egg
}

func convertTriangle(arr []json.RawMessage) string {
	var params [1]float64
	json.Unmarshal(arr[1], &params)

	var op string
	json.Unmarshal(arr[2], &op)

	var opParams [3]float64
	json.Unmarshal(arr[3], &opParams)

	tri := fmt.Sprintf("triangle(%s)", f(params[0]*2))

	if op == "Extrude" {
		return tri + ".extrude(" + f(opParams[0]*2) + ")"
	}
	return tri
}

func parseBlend(raw json.RawMessage) (string, float64) {
	var arr []json.RawMessage
	json.Unmarshal(raw, &arr)
	if len(arr) < 2 {
		return "add", 0
	}
	var mode string
	var radius float64
	json.Unmarshal(arr[0], &mode)
	json.Unmarshal(arr[1], &radius)
	return mode, radius
}

func blendOp(mode string, radius float64) string {
	if radius < 1e-8 {
		switch mode {
		case "add":
			return "|"
		case "sub":
			return "-"
		case "mat":
			return "|@0"
		case "rep":
			return "|!0"
		case "avo":
			return "|^0"
		case "int":
			return "&"
		}
		return "|"
	}
	r := f(radius)
	switch mode {
	case "add":
		return "|~" + r
	case "sub":
		return "-~" + r
	case "mat":
		return "|@" + r
	case "rep":
		return "|!" + r
	case "avo":
		return "|^" + r
	case "int":
		return "&~" + r
	}
	return "|~" + r
}

func parseRepeat(raw json.RawMessage) (string, []float64) {
	var arr []json.RawMessage
	json.Unmarshal(raw, &arr)
	if len(arr) == 0 {
		return "None", nil
	}
	var mode string
	json.Unmarshal(arr[0], &mode)
	if mode == "None" {
		return "None", nil
	}
	var nums []float64
	for _, r := range arr[1:] {
		var v float64
		json.Unmarshal(r, &v)
		nums = append(nums, v)
	}
	return mode, nums
}

func getGroupRef(prim json.RawMessage) string {
	var arr []json.RawMessage
	json.Unmarshal(prim, &arr)
	if len(arr) == 1 {
		var name string
		json.Unmarshal(arr[0], &name)
		return name
	}
	return ""
}

func convertElement(elem Element, materials []Material) string {
	// Material color
	colorHex := "#888888"
	for _, m := range materials {
		if m.UUID == elem.Material {
			c := m.PBR.Color
			colorHex = rgbToHex(c[0], c[1], c[2])
			break
		}
	}

	var expr string

	if elem.RM {
		expr = convertPrim(elem.Prim)
		if expr == "" {
			return ""
		}
	} else {
		ref := getGroupRef(elem.Prim)
		if ref == "" {
			return ""
		}
		expr = "grp_" + strings.ReplaceAll(strings.ToLower(ref), " ", "_")
	}

	// Repeat
	mode, nums := parseRepeat(elem.Repeat)
	if mode == "XYZ" && len(nums) >= 6 {
		cy := int(nums[1])
		sy := nums[4]
		if cy > 1 {
			expr += fmt.Sprintf(".rep(y: %s, count: %d)", f(sy), cy)
		}
	} else if mode == "ANG" && len(nums) >= 3 {
		count := int(nums[1])
		angOffset := nums[2]
		expr += fmt.Sprintf(".at(x: %s).array(%d, radius: 0)", f(angOffset), count)
	}

	// Quaternion
	loc := elem.Location
	qx, qy, qz, qw := loc[5], loc[6], loc[7], loc[8]
	if !nearIdentityQuat(qx, qy, qz, qw) {
		expr += fmt.Sprintf(".quat(%s, %s, %s, %s)", f(qx), f(qy), f(qz), f(qw))
	}

	// Translation
	tx, ty, tz := loc[0], loc[1], loc[2]
	if !nearZero3(tx, ty, tz) {
		expr += fmt.Sprintf(".at(%s, %s, %s)", f(tx), f(ty), f(tz))
	}

	// Color
	expr += ".color(" + colorHex + ")"

	return expr
}

func convertGroup(group Group, materials []Material) string {
	var parts []string

	for i, elem := range group.Elements {
		expr := convertElement(elem, materials)
		if expr == "" {
			continue
		}

		if i == 0 || len(parts) == 0 {
			parts = append(parts, expr)
		} else {
			mode, radius := parseBlend(elem.Blend)
			op := blendOp(mode, radius)
			parts = append(parts, op+" "+expr)
		}
	}

	return strings.Join(parts, "\n")
}

func convertScene(scene Scene) string {
	var out strings.Builder

	fmt.Fprintf(&out, "// Converted from IQ SDF editor: %s\n\n", scene.Info.Name)

	// Named groups as variables
	for _, group := range scene.Model.Groups {
		if group.Name == "" {
			continue
		}
		safeName := "grp_" + strings.ReplaceAll(strings.ToLower(group.Name), " ", "_")
		body := convertGroup(group, scene.Materials)
		if body != "" {
			fmt.Fprintf(&out, "%s = {\n%s\n}\n\n", safeName, body)
		}
	}

	// Root group
	for _, group := range scene.Model.Groups {
		if group.Name == "" {
			body := convertGroup(group, scene.Materials)
			if body != "" {
				fmt.Fprintln(&out, body)
			}
			break
		}
	}

	// Background
	if scene.Environment.Background.Type == "solid" {
		c := scene.Environment.Background.Data.Color
		fmt.Fprintf(&out, "\nbg #%02x%02x%02x\n", int(c[0]), int(c[1]), int(c[2]))
	}

	// Lighting
	for _, light := range scene.Lighting.Lights {
		if light.Type == "directional" && light.Data.ShadowEnable {
			d := light.Data.Direction
			var ambient float64 = 0.4
			for _, l2 := range scene.Lighting.Lights {
				if l2.Type == "ambient" {
					ambient = l2.Data.Intensity
				}
			}
			fmt.Fprintf(&out, "\nlight {\n  sun {\n    dir: [%s, %s, %s]\n    intensity: %s\n    shadows: true\n  }\n  ambient: %s\n}\n",
				f(d[0]), f(d[1]), f(d[2]), f(light.Data.Intensity), f(ambient))
			break
		}
	}

	return out.String()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: json2chisel <scene.json>")
		os.Exit(1)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	var scene Scene
	if err := json.Unmarshal(data, &scene); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	chisel := convertScene(scene)

	// Format through the Chisel formatter for clean output
	formatted, err := format.Format(chisel)
	if err != nil {
		// If formatting fails, output raw (might have unsupported features)
		fmt.Fprint(os.Stdout, chisel)
		fmt.Fprintf(os.Stderr, "Warning: formatter error: %v\n", err)
		return
	}

	fmt.Fprint(os.Stdout, formatted)
}
