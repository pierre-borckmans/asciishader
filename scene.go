package main

import "math"

// Scene is a distance function + description
type Scene struct {
	Name  string
	SDF   func(p Vec3, time float64) float64
	Color func(p Vec3, time float64) Vec3 // material color at world point (RGB 0-1), nil = white
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
		Name: "Gyroid",
		SDF:  sceneGyroid,
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
