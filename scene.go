package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// Scene is a distance function + description
type Scene struct {
	Name  string
	SDF   func(p Vec3, time float64) float64
	Color func(p Vec3, time float64) Vec3 // material color at world point (RGB 0-1), nil = white
	GLSL  string                          // optional GLSL code for GPU editor (sceneSDF + sceneColor)
}

var scenes = []Scene{
	{
		Name:  "Bullet Train",
		SDF:   sceneTrain,
		Color: colorTrain,
	},
	{
		Name:  "Sphere & Cube",
		SDF:   sceneSphereAndCube,
		Color: colorSphereAndCube,
	},
	{
		Name: "Torus Knot",
		SDF:  sceneTorusKnot,
	},
	{
		Name: "Morphing Shapes",
		SDF:  sceneMorph,
	},
	{
		Name: "Infinite Pillars",
		SDF:  scenePillars,
	},
	{
		Name: "Alien Egg",
		SDF:  sceneAlienEgg,
	},
	{
		Name:  "Alien Egg Color",
		SDF:   sceneAlienEggColor,
		Color: colorAlienEgg,
	},
	{
		Name: "Gyroid",
		SDF:  sceneGyroid,
	},
	{
		Name:  "Crystal Cluster",
		SDF:   sceneCrystal,
		Color: colorCrystal,
	},
	{
		Name:  "Plasma Orb",
		SDF:   scenePlasma,
		Color: colorPlasma,
	},
	{
		Name:  "Plasma Rainbow",
		SDF:   scenePlasma,
		Color: colorPlasmaRainbow,
	},
	{
		Name:  "Deep Nebula",
		SDF:   sceneNebula,
		Color: colorNebula,
	},
	{
		Name:  "Solar Flare",
		SDF:   sceneSolarFlare,
		Color: colorSolarFlare,
	},
	{
		Name:  "Void Bloom",
		SDF:   sceneVoidBloom,
		Color: colorVoidBloom,
	},
	{
		Name:  "Jellyfish",
		SDF:   sceneJellyfish,
		Color: colorJellyfish,
	},
	{
		Name:  "Frozen Star",
		SDF:   sceneFrozenStar,
		Color: colorFrozenStar,
	},
	{
		Name:  "Railway Express",
		SDF:   sceneTrain,
		Color: colorTrain,
	},
	{
		Name:  "Lava Lamp",
		SDF:   scenePlasma,
		Color: colorPlasma,
	},
	{
		Name:  "Mercury",
		SDF:   scenePlasma,
		Color: colorPlasma,
	},
	{
		Name:  "Amoeba",
		SDF:   scenePlasma,
		Color: colorPlasma,
	},
}

// Scene: Bullet train coming at us
func sceneTrain(p Vec3, _ float64) float64 {
	// === TRAIN BODY ===
	// Main fuselage — rounded box extending back along +Z
	bodyP := p.Sub(V(0, 0.4, 3.5))
	body := opRound(sdBox(bodyP, V(1.0, 0.85, 4.0)), 0.2)

	// Nose — elongated sphere blended into front of body
	noseP := p.Sub(V(0, 0.2, 0))
	noseStretched := V(noseP.X, noseP.Y, noseP.Z*0.5)
	nose := sdSphere(noseStretched, 1.0)
	train := opSmoothUnion(body, nose, 0.6)

	// Windshield — recessed box at upper front
	wsP := p.Sub(V(0, 0.9, 0.3))
	windshield := sdBox(wsP, V(0.5, 0.2, 0.12))
	train = opSubtract(train, windshield)

	// Headlights — two small bumps at lower front
	hl1 := sdSphere(p.Sub(V(-0.6, -0.15, -0.7)), 0.1)
	hl2 := sdSphere(p.Sub(V(0.6, -0.15, -0.7)), 0.1)
	train = opUnion(train, opUnion(hl1, hl2))

	// Side stripe — subtle groove along the body
	stripeP := p.Sub(V(0, 0.4, 3.0))
	stripe := sdBox(stripeP, V(1.25, 0.03, 4.5))
	train = opSubtract(train, stripe)

	// === UNDERCARRIAGE ===
	// Front bogie
	bogie1 := opRound(sdBox(p.Sub(V(0, -0.65, 1.0)), V(0.8, 0.12, 0.4)), 0.05)
	// Rear bogie
	bogie2 := opRound(sdBox(p.Sub(V(0, -0.65, 5.5)), V(0.8, 0.12, 0.4)), 0.05)
	train = opUnion(train, opUnion(bogie1, bogie2))

	// === TRACKS ===
	// Two rails extending toward camera and beyond
	rail1 := sdBox(p.Sub(V(-0.7, -0.88, 0)), V(0.03, 0.04, 30.0))
	rail2 := sdBox(p.Sub(V(0.7, -0.88, 0)), V(0.03, 0.04, 30.0))

	// Railroad ties — repeated along Z
	tieP := p.Sub(V(0, -0.93, 0))
	rep := 0.7
	tieP.Z = math.Mod(tieP.Z+rep*0.5, rep) - rep*0.5
	ties := sdBox(tieP, V(1.1, 0.02, 0.06))

	tracks := opUnion(opUnion(rail1, rail2), ties)

	// === GROUND ===
	ground := sdPlane(p, V(0, 1, 0), 1.0)

	return opUnion(train, opUnion(tracks, ground))
}

// Scene: Static sphere and cube
func sceneSphereAndCube(p Vec3, _ float64) float64 {
	sphere := sdSphere(p.Sub(V(-1.2, 0, 0)), 0.9)
	cube := sdBox(p.Sub(V(1.2, 0, 0)), V(0.7, 0.7, 0.7))
	return opUnion(sphere, cube)
}

// Scene 0: Spinning torus
func sceneTorusKnot(p Vec3, t float64) float64 {
	// Rotate the whole scene
	p = p.RotateY(t * 0.5)
	p = p.RotateX(t * 0.3)

	// Main torus
	d1 := sdTorus(p, 1.2, 0.4)

	// Orbiting spheres
	for i := 0; i < 5; i++ {
		angle := t*1.5 + float64(i)*math.Pi*2/5
		offset := V(math.Cos(angle)*1.2, math.Sin(angle*1.7)*0.4, math.Sin(angle)*1.2)
		d1 = opSmoothUnion(d1, sdSphere(p.Sub(offset), 0.25), 0.3)
	}

	// Ground plane (subtle)
	d2 := sdPlane(p, V(0, 1, 0), 2.0)

	return opUnion(d1, d2)
}

// Scene 1: Morphing between shapes
func sceneMorph(p Vec3, t float64) float64 {
	p = p.RotateY(t * 0.4)

	// Morph factor oscillates 0..1
	morph := (math.Sin(t*0.8) + 1) * 0.5

	d1 := sdBox(p, V(0.9, 0.9, 0.9))
	d2 := sdSphere(p, 1.2)
	d3 := sdOctahedron(p, 1.4)

	// Blend between three shapes
	var d float64
	if morph < 0.5 {
		f := morph * 2
		d = mix(d1, d2, f)
	} else {
		f := (morph - 0.5) * 2
		d = mix(d2, d3, f)
	}

	// Floor
	floor := sdPlane(p, V(0, 1, 0), 2.0)
	return opUnion(d, floor)
}

// Scene 2: Infinite pillars
func scenePillars(p Vec3, t float64) float64 {
	// Move camera through the scene
	p.Z += t * 2.0

	// Repeated columns
	rp := opRepXZ(p, 4.0, 4.0)
	pillars := sdCylinder(rp, 0.5, 8.0)

	// Floor and ceiling
	floor := sdPlane(p, V(0, 1, 0), 2.0)
	ceil := sdPlane(p, V(0, -1, 0), 6.0)

	// Floating orb
	orbPos := V(math.Sin(t)*1.5, math.Sin(t*1.3)*0.5, math.Cos(t)*1.5+2)
	orb := sdSphere(p.Sub(orbPos), 0.4)

	return opUnion(opUnion(pillars, opUnion(floor, ceil)), orb)
}

// Scene 3: Alien egg
func sceneAlienEgg(p Vec3, t float64) float64 {
	p = p.RotateY(t * 0.3)

	// Egg shape (stretched sphere)
	ep := V(p.X, p.Y*0.7, p.Z)
	egg := sdSphere(ep, 1.0)

	// Pulsating displacement
	disp := math.Sin(p.X*5+t*2) * math.Sin(p.Y*5+t*1.5) * math.Sin(p.Z*5+t*1.8) * 0.08
	egg += disp

	// Inner glow sphere (carved out)
	inner := sdSphere(p, 0.6+math.Sin(t*2)*0.1)
	shell := opSubtract(egg, inner)

	// Base
	base := sdTorus(V(p.X, p.Y+1.1, p.Z), 0.6, 0.15)

	// Floor
	floor := sdPlane(p, V(0, 1, 0), 1.5)

	return opUnion(opUnion(shell, base), floor)
}

// Scene 4: Gyroid surface
func sceneGyroid(p Vec3, t float64) float64 {
	p = p.RotateY(t * 0.2)
	p = p.RotateX(t * 0.15)

	scale := 3.0
	sp := p.Mul(scale)

	// Gyroid implicit surface
	gyroid := math.Sin(sp.X)*math.Cos(sp.Y) +
		math.Sin(sp.Y)*math.Cos(sp.Z) +
		math.Sin(sp.Z)*math.Cos(sp.X)
	gyroid = math.Abs(gyroid) / scale

	// Bound it in a sphere
	bound := sdSphere(p, 2.0)

	return opIntersect(gyroid-0.03, bound)
}

// --- Material color functions ---

// colorTrain returns material color for the bullet train scene.
func colorTrain(p Vec3, _ float64) Vec3 {
	// Windshield — tinted blue-green glass
	wsP := p.Sub(V(0, 0.9, 0.3))
	if math.Abs(wsP.X) < 0.55 && math.Abs(wsP.Y) < 0.25 && math.Abs(wsP.Z) < 0.2 {
		return V(0.4, 0.6, 0.7)
	}

	// Headlights — bright white
	if sdSphere(p.Sub(V(-0.6, -0.15, -0.7)), 0.15) < 0 ||
		sdSphere(p.Sub(V(0.6, -0.15, -0.7)), 0.15) < 0 {
		return V(1.0, 1.0, 0.9)
	}

	// Bogies — dark grey
	bogie1 := opRound(sdBox(p.Sub(V(0, -0.65, 1.0)), V(0.85, 0.15, 0.45)), 0.05)
	bogie2 := opRound(sdBox(p.Sub(V(0, -0.65, 5.5)), V(0.85, 0.15, 0.45)), 0.05)
	if math.Min(bogie1, bogie2) < 0.02 {
		return V(0.25, 0.25, 0.28)
	}

	// Rails — silver
	rail1 := sdBox(p.Sub(V(-0.7, -0.88, 0)), V(0.05, 0.06, 30.0))
	rail2 := sdBox(p.Sub(V(0.7, -0.88, 0)), V(0.05, 0.06, 30.0))
	if math.Min(rail1, rail2) < 0.02 {
		return V(0.7, 0.72, 0.75)
	}

	// Ties — brown wood
	tieP := p.Sub(V(0, -0.93, 0))
	rep := 0.7
	tieP.Z = math.Mod(tieP.Z+rep*0.5, rep) - rep*0.5
	if sdBox(tieP, V(1.15, 0.04, 0.08)) < 0.02 {
		return V(0.45, 0.3, 0.15)
	}

	// Ground — dark earth
	if p.Y < -0.9 {
		return V(0.3, 0.28, 0.22)
	}

	// Train body — light grey with blue stripe hint
	return V(0.78, 0.8, 0.84)
}

// colorSphereAndCube returns red for sphere, blue for cube.
func colorSphereAndCube(p Vec3, _ float64) Vec3 {
	sphere := sdSphere(p.Sub(V(-1.2, 0, 0)), 0.9)
	cube := sdBox(p.Sub(V(1.2, 0, 0)), V(0.7, 0.7, 0.7))
	if sphere < cube {
		return V(0.9, 0.15, 0.1)
	}
	return V(0.1, 0.2, 0.9)
}

// Scene: Alien egg without floor, colored
func sceneAlienEggColor(p Vec3, t float64) float64 {
	p = p.RotateY(t * 0.3)

	// Egg shape (stretched sphere)
	ep := V(p.X, p.Y*0.7, p.Z)
	egg := sdSphere(ep, 1.0)

	// Pulsating displacement
	disp := math.Sin(p.X*5+t*2) * math.Sin(p.Y*5+t*1.5) * math.Sin(p.Z*5+t*1.8) * 0.08
	egg += disp

	// Inner glow sphere (carved out)
	inner := sdSphere(p, 0.6+math.Sin(t*2)*0.1)
	shell := opSubtract(egg, inner)

	// Base
	base := sdTorus(V(p.X, p.Y+1.1, p.Z), 0.6, 0.15)

	return opUnion(shell, base)
}

// colorAlienEgg returns material color for the alien egg scene.
func colorAlienEgg(p Vec3, t float64) Vec3 {
	p = p.RotateY(t * 0.3)

	// Base torus — warm bronze
	base := sdTorus(V(p.X, p.Y+1.1, p.Z), 0.6, 0.15)
	if base < 0.05 {
		return V(0.7, 0.5, 0.25)
	}

	// Shell — organic color that shifts with the displacement pattern
	wave := math.Sin(p.X*5+t*2) * math.Sin(p.Y*5+t*1.5) * math.Sin(p.Z*5+t*1.8)
	// Map wave (-1..1) to a green-purple gradient
	f := wave*0.5 + 0.5 // 0..1
	r := 0.5 + 0.5*f
	g := 0.85 - 0.3*f
	b := 0.5 + 0.4*f
	return V(r, g, b)
}

// --- Crystal Cluster ---

func sceneCrystal(p Vec3, t float64) float64 {
	p = p.RotateY(t * 0.2)

	// Central crystal — tall octahedron
	c0 := sdOctahedron(V(p.X, p.Y*0.6, p.Z), 1.2)

	// Tilted side crystals
	p1 := p.Sub(V(0.8, -0.3, 0.4)).RotateX(0.4)
	c1 := sdOctahedron(V(p1.X, p1.Y*0.5, p1.Z), 0.7)

	p2 := p.Sub(V(-0.6, -0.2, -0.5)).RotateX(-0.3).RotateY(1.0)
	c2 := sdOctahedron(V(p2.X, p2.Y*0.5, p2.Z), 0.6)

	p3 := p.Sub(V(-0.3, 0.5, 0.7)).RotateX(0.6).RotateY(-0.8)
	c3 := sdOctahedron(V(p3.X, p3.Y*0.45, p3.Z), 0.5)

	p4 := p.Sub(V(0.5, -0.5, -0.6)).RotateX(-0.5).RotateY(2.0)
	c4 := sdOctahedron(V(p4.X, p4.Y*0.5, p4.Z), 0.55)

	d := opSmoothUnion(c0, c1, 0.15)
	d = opSmoothUnion(d, c2, 0.15)
	d = opSmoothUnion(d, c3, 0.15)
	d = opSmoothUnion(d, c4, 0.15)
	return d
}

func colorCrystal(p Vec3, t float64) Vec3 {
	p = p.RotateY(t * 0.2)

	// Prismatic rainbow based on vertical position + angle
	angle := math.Atan2(p.Z, p.X)
	h := (p.Y*0.4 + angle*0.3 + t*0.2)
	// HSV-like hue to RGB (full saturation, full value)
	return hueToRGB(h)
}

// --- Plasma Orb ---

func scenePlasma(p Vec3, t float64) float64 {
	p = p.RotateY(t * 0.4)
	p = p.RotateX(t * 0.15)

	// Base sphere
	d := sdSphere(p, 1.3)

	// Layered displacement — two frequencies for organic turbulence
	disp1 := math.Sin(p.X*4+t*1.5) * math.Cos(p.Y*3+t*1.2) * math.Sin(p.Z*4+t*1.8) * 0.15
	disp2 := math.Sin(p.X*8+t*2.5) * math.Sin(p.Y*7-t*2.0) * math.Cos(p.Z*6+t*1.3) * 0.06
	d += disp1 + disp2

	// Carved inner void
	inner := sdSphere(p, 0.5+math.Sin(t*1.5)*0.15)
	d = opSubtract(d, inner)

	// Orbiting sparks
	for i := 0; i < 3; i++ {
		a := t*1.2 + float64(i)*math.Pi*2/3
		sp := V(math.Cos(a)*1.6, math.Sin(a*0.7)*0.4, math.Sin(a)*1.6)
		d = opSmoothUnion(d, sdSphere(p.Sub(sp), 0.15), 0.3)
	}

	return d
}

func colorPlasma(p Vec3, t float64) Vec3 {
	p = p.RotateY(t * 0.4)
	p = p.RotateX(t * 0.15)

	// Electric aurora: cyan core, magenta edges, white-hot sparks
	dist := p.Length()
	wave := math.Sin(p.X*4+t*1.5)*math.Cos(p.Y*3+t*1.2) + math.Sin(p.Z*4+t*1.8)
	f := wave*0.25 + 0.5

	// Close to center = hot cyan/white, far = magenta/purple
	r := 0.4 + 0.6*f
	g := 0.3 + 0.7*(1-f)
	b := 0.8 + 0.2*math.Sin(dist*3+t)
	return V(clamp(r, 0, 1), clamp(g, 0, 1), clamp(b, 0, 1))
}

func colorPlasmaRainbow(p Vec3, t float64) Vec3 {
	p = p.RotateY(t * 0.4)
	p = p.RotateX(t * 0.15)

	// Full rainbow cycling through the displacement pattern
	wave := math.Sin(p.X*4+t*1.5)*math.Cos(p.Y*3+t*1.2) + math.Sin(p.Z*4+t*1.8)
	angle := math.Atan2(p.Z, p.X)
	dist := p.Length()

	// Hue sweeps with surface angle, depth, and time
	h := angle + wave*0.8 + dist*1.5 + t*0.6
	col := hueToRGB(h)

	// Brighten at peaks of displacement, slightly dim in troughs
	bright := 0.6 + 0.4*clamp(wave*0.5+0.5, 0, 1)
	return V(col.X * bright, col.Y * bright, col.Z * bright)
}

// --- Deep Nebula ---

func sceneNebula(p Vec3, t float64) float64 {
	p = p.RotateY(t * 0.15)
	p = p.RotateX(t * 0.1)

	// Gyroid surface — cosmic gas structure
	scale := 2.5
	sp := p.Mul(scale)
	gyroid := math.Sin(sp.X)*math.Cos(sp.Y) +
		math.Sin(sp.Y)*math.Cos(sp.Z) +
		math.Sin(sp.Z)*math.Cos(sp.X)
	gyroid = math.Abs(gyroid)/scale - 0.03

	// Slow breathing
	breath := 1.0 + math.Sin(t*0.5)*0.08
	bound := sdSphere(p, 1.8*breath)

	nebula := opIntersect(gyroid, bound)

	// Dense core — small sphere at center
	core := sdSphere(p, 0.3+math.Sin(t*0.8)*0.05)

	return opSmoothUnion(nebula, core, 0.2)
}

func colorNebula(p Vec3, t float64) Vec3 {
	p = p.RotateY(t * 0.15)
	p = p.RotateX(t * 0.1)

	dist := p.Length()

	// Core = bright gold/white, outer = deep purple/blue
	coreFade := math.Exp(-dist * 2.0)

	// Swirling hue variation
	angle := math.Atan2(p.Z, p.X) + math.Sin(p.Y*3+t)*0.5
	swirl := math.Sin(angle*2+t*0.3)*0.5 + 0.5

	r := 0.3 + 0.7*coreFade + 0.3*swirl*(1-coreFade)
	g := 0.1 + 0.8*coreFade
	b := 0.5 + 0.5*(1-coreFade) - 0.2*coreFade
	return V(clamp(r, 0, 1), clamp(g, 0, 1), clamp(b, 0, 1))
}

// --- Solar Flare ---

func sceneSolarFlare(p Vec3, t float64) float64 {
	p = p.RotateY(t * 0.25)

	// Sun core
	d := sdSphere(p, 1.0)

	// Violent surface displacement — solar granulation
	disp := math.Sin(p.X*6+t*3) * math.Sin(p.Y*5+t*2.5) * math.Sin(p.Z*7+t*2) * 0.12
	disp += math.Sin(p.X*3-t*1.5) * math.Cos(p.Z*3+t*1.8) * 0.08
	d += disp

	// Prominences — arching loops of plasma
	for i := 0; i < 4; i++ {
		a := float64(i)*math.Pi*0.5 + t*0.3
		// Arc center
		cx, cz := math.Cos(a)*1.1, math.Sin(a)*1.1
		arcP := p.Sub(V(cx, 0, cz))
		// Torus-like arc rising from surface
		arc := sdTorus(arcP.RotateX(math.Pi/2+math.Sin(t*0.5+float64(i))*0.3), 0.35, 0.06+math.Sin(t*2+float64(i))*0.02)
		d = opSmoothUnion(d, arc, 0.2)
	}

	// Floating embers
	for i := 0; i < 5; i++ {
		a := t*0.8 + float64(i)*math.Pi*2/5
		r := 2.0 + math.Sin(t*0.5+float64(i)*1.3)*0.4
		sp := V(math.Cos(a)*r, math.Sin(a*0.6+float64(i))*0.8, math.Sin(a)*r)
		d = opSmoothUnion(d, sdSphere(p.Sub(sp), 0.08+math.Sin(t*3+float64(i))*0.03), 0.15)
	}

	return d
}

func colorSolarFlare(p Vec3, t float64) Vec3 {
	p = p.RotateY(t * 0.25)
	dist := p.Length()

	// Temperature gradient: white-hot core → yellow → orange → deep red at edges
	temp := clamp(1.5-dist*0.8, 0, 1)
	// Flickering variation
	flicker := math.Sin(p.X*6+t*3)*math.Sin(p.Y*5+t*2.5)*0.15 + 0.85

	r := 1.0
	g := 0.3 + 0.7*temp*temp
	b := temp * temp * temp * 0.6
	return V(clamp(r*flicker, 0, 1), clamp(g*flicker, 0, 1), clamp(b*flicker, 0, 1))
}

// --- Void Bloom ---

func sceneVoidBloom(p Vec3, t float64) float64 {
	p = p.RotateY(t * 0.3)
	p = p.RotateX(t * 0.12)

	// Multiple shells blooming outward at different phases
	d := maxDist

	for i := 0; i < 3; i++ {
		phase := t*0.6 + float64(i)*math.Pi*2/3
		bloom := (math.Sin(phase)*0.5 + 0.5) // 0..1
		radius := 0.5 + bloom*0.8
		thickness := 0.04 + bloom*0.06

		shell := math.Abs(sdSphere(p, radius)) - thickness

		// Cut petals into each shell
		petalCut := math.Sin(p.X*5+float64(i)) * math.Sin(p.Y*5+float64(i)*2) * math.Sin(p.Z*5+float64(i)*3) * 0.1
		shell += petalCut

		d = opSmoothUnion(d, shell, 0.1)
	}

	// Central seed
	seed := sdSphere(p, 0.2+math.Sin(t*2)*0.05)
	d = opSmoothUnion(d, seed, 0.15)

	// Floating pollen
	for i := 0; i < 6; i++ {
		a := float64(i)*math.Pi*2/6 + t*0.5
		r := 1.8 + math.Sin(t*0.7+float64(i)*0.9)*0.3
		h := math.Sin(t*0.4+float64(i)*1.1) * 0.6
		sp := V(math.Cos(a)*r, h, math.Sin(a)*r)
		d = opSmoothUnion(d, sdSphere(p.Sub(sp), 0.06), 0.08)
	}

	return d
}

func colorVoidBloom(p Vec3, t float64) Vec3 {
	p = p.RotateY(t * 0.3)
	p = p.RotateX(t * 0.12)

	dist := p.Length()
	angle := math.Atan2(p.Z, p.X)

	// Full rainbow spectrum that rotates with time
	h := angle + p.Y*1.5 + t*0.4
	base := hueToRGB(h)

	// Brighten near center, darken far out
	bright := clamp(1.3-dist*0.4, 0.4, 1.0)
	return V(base.X*bright, base.Y*bright, base.Z*bright)
}

// --- Jellyfish ---

func sceneJellyfish(p Vec3, t float64) float64 {
	p = p.RotateY(t * 0.2)

	// Breathing motion
	breath := math.Sin(t*1.2) * 0.15

	// Bell — squashed sphere, open at bottom
	bellP := V(p.X, (p.Y-0.3-breath*0.3)*0.65, p.Z)
	bell := sdSphere(bellP, 1.0)
	// Carve out the inside
	innerBell := sdSphere(V(p.X, (p.Y-0.15)*0.7, p.Z), 0.85)
	bell = opSubtract(bell, innerBell)
	// Cut off the bottom half opening
	bell = opIntersect(bell, sdPlane(p, V(0, -1, 0), 0.2+breath*0.2))

	// Undulating bell edge displacement
	edgeWave := math.Sin(math.Atan2(p.Z, p.X)*8+t*2) * 0.04 * clamp(1-p.Y, 0, 1)
	bell += edgeWave

	d := bell

	// Tentacles — several capsules hanging down with wave motion
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi * 2 / 8
		r := 0.5 + math.Sin(float64(i)*1.7)*0.15
		baseX := math.Cos(a) * r
		baseZ := math.Sin(a) * r

		// Each tentacle sways
		sway := math.Sin(t*1.5+float64(i)*0.8) * 0.3
		swayZ := math.Cos(t*1.2+float64(i)*1.1) * 0.2
		length := 1.2 + math.Sin(float64(i)*2.3)*0.4

		top := V(baseX, -0.2, baseZ)
		bot := V(baseX+sway, -0.2-length, baseZ+swayZ)
		tent := sdCapsule(p, top, bot, 0.02+math.Sin(float64(i))*0.01)
		d = opSmoothUnion(d, tent, 0.08)
	}

	// Oral arms — thicker, shorter central tentacles
	for i := 0; i < 4; i++ {
		a := float64(i)*math.Pi*0.5 + 0.3
		sway := math.Sin(t*1.0+float64(i)*1.5) * 0.15
		top := V(math.Cos(a)*0.15, -0.1, math.Sin(a)*0.15)
		bot := V(math.Cos(a)*0.15+sway, -0.8, math.Sin(a)*0.15)
		arm := sdCapsule(p, top, bot, 0.05)
		d = opSmoothUnion(d, arm, 0.1)
	}

	return d
}

func colorJellyfish(p Vec3, t float64) Vec3 {
	p = p.RotateY(t * 0.2)

	// Bell vs tentacles — by height
	if p.Y > -0.2 {
		// Bell — translucent blue-pink with bioluminescent pulse
		pulse := math.Sin(t*2+p.Y*3)*0.3 + 0.7
		angle := math.Atan2(p.Z, p.X)
		wave := math.Sin(angle*4+t*1.5)*0.15 + 0.85
		r := 0.4 * pulse * wave
		g := 0.5 * pulse
		b := 0.95 * pulse
		return V(clamp(r, 0, 1), clamp(g, 0, 1), clamp(b, 0, 1))
	}

	// Tentacles — rainbow bioluminescence that pulses down the length
	depth := clamp((-p.Y-0.2)*0.8, 0, 1)
	h := depth*math.Pi*2 + t*0.8 + math.Atan2(p.Z, p.X)*0.5
	col := hueToRGB(h)
	glow := math.Sin(depth*8-t*3)*0.3 + 0.7
	return V(col.X * glow, col.Y * glow, col.Z * glow)
}

// --- Frozen Star ---

func sceneFrozenStar(p Vec3, t float64) float64 {
	p = p.RotateY(t * 0.15)
	p = p.RotateX(t * 0.08)

	// Spiky core — sphere with sharp displacement
	d := sdSphere(p, 0.9)
	// Sharp crystalline spikes
	spike := math.Abs(math.Sin(p.X*8)*math.Sin(p.Y*8)*math.Sin(p.Z*8)) * 0.2
	d -= spike

	// Ice ring
	ring := sdTorus(p, 1.6, 0.05+math.Sin(t+p.X*4)*0.02)
	d = opSmoothUnion(d, ring, 0.1)

	// Second tilted ring
	rp2 := p.RotateX(math.Pi / 3)
	ring2 := sdTorus(rp2, 1.4, 0.04+math.Cos(t*0.8+rp2.Z*5)*0.015)
	d = opSmoothUnion(d, ring2, 0.1)

	// Orbiting fragments — broken shards
	for i := 0; i < 7; i++ {
		a := float64(i)*math.Pi*2/7 + t*0.4
		r := 2.0 + math.Sin(t*0.3+float64(i)*1.5)*0.3
		h := math.Sin(t*0.25+float64(i)*0.9) * 0.8
		fp := p.Sub(V(math.Cos(a)*r, h, math.Sin(a)*r))
		// Small rotated boxes as shards
		fp = fp.RotateY(t + float64(i))
		shard := sdBox(fp, V(0.06, 0.1, 0.04))
		d = opUnion(d, shard)
	}

	return d
}

func colorFrozenStar(p Vec3, t float64) Vec3 {
	p = p.RotateY(t * 0.15)
	p = p.RotateX(t * 0.08)

	dist := p.Length()

	// Ice blue core, prismatic edges where spikes refract
	spike := math.Sin(p.X*8) * math.Sin(p.Y*8) * math.Sin(p.Z*8)

	// Core: cold white-blue
	r := 0.6 + 0.4*math.Abs(spike)
	g := 0.8 + 0.2*math.Abs(spike)
	b := 1.0

	// Orbiting shards and rings get rainbow refractions
	if dist > 1.2 {
		h := math.Atan2(p.Z, p.X) + p.Y*2 + t*0.5
		rainbow := hueToRGB(h)
		f := clamp((dist-1.2)*1.5, 0, 1)
		r = mix(r, rainbow.X*0.9+0.1, f)
		g = mix(g, rainbow.Y*0.9+0.1, f)
		b = mix(b, rainbow.Z*0.9+0.1, f)
	}

	return V(clamp(r, 0, 1), clamp(g, 0, 1), clamp(b, 0, 1))
}

// hueToRGB converts a hue value (any float, wraps) to an RGB color.
func hueToRGB(h float64) Vec3 {
	h = math.Mod(h, math.Pi*2)
	if h < 0 {
		h += math.Pi * 2
	}
	h = h / (math.Pi * 2) * 6 // 0..6
	i := int(h)
	f := h - float64(i)
	switch i % 6 {
	case 0:
		return V(1, f, 0)
	case 1:
		return V(1-f, 1, 0)
	case 2:
		return V(0, 1, f)
	case 3:
		return V(0, 1-f, 1)
	case 4:
		return V(f, 0, 1)
	default:
		return V(1, 0, 1-f)
	}
}

// loadShaderFiles scans the shaders/ directory for .glsl files and associates
// them with scenes. For each file, if a built-in scene with a matching name
// exists, its GLSL field is set. Otherwise a new GPU-only scene is appended
// (using scenePlasma/colorPlasma as CPU fallback).
//
// File naming: snake_case.glsl → "Title Case" scene name.
// A "// Scene: Custom Name" header line overrides the filename-derived name.
func loadShaderFiles() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	dir := filepath.Dir(exePath)
	pattern := filepath.Join(dir, "shaders", "*.glsl")

	// Also try current working directory
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		cwdPattern := filepath.Join("shaders", "*.glsl")
		matches, err = filepath.Glob(cwdPattern)
		if err != nil || len(matches) == 0 {
			return
		}
	}

	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot read %s: %v\n", path, err)
			continue
		}
		content := string(data)
		name := shaderFileName(path, content)

		// Try to match an existing scene by name
		found := false
		for i := range scenes {
			if scenes[i].Name == name {
				scenes[i].GLSL = content
				found = true
				break
			}
		}
		if !found {
			// New GPU-only scene with CPU fallback
			scenes = append(scenes, Scene{
				Name:  name,
				SDF:   scenePlasma,
				Color: colorPlasma,
				GLSL:  content,
			})
		}
	}
}

// shaderFileName derives a scene name from a .glsl file path.
// If the file contains a "// Scene: Name" header, that name is used.
// Otherwise the filename is converted from snake_case to Title Case.
func shaderFileName(path, content string) string {
	// Check for "// Scene: ..." header in first 5 lines
	lines := strings.SplitN(content, "\n", 6)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "// Scene:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "// Scene:"))
			if name != "" {
				return name
			}
		}
	}

	// Fall back to filename → title case
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".glsl")
	words := strings.Split(base, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
