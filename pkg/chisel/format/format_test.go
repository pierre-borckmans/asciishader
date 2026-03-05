package format

import (
	"testing"
)

func TestFormatUnionNoSpaces(t *testing.T) {
	got, err := Format("sphere|box")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "sphere | box\n"
	if got != want {
		t.Errorf("Format(\"sphere|box\")\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatMethodCallSpaces(t *testing.T) {
	got, err := Format("sphere.at(2,0,0)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "sphere.at(2, 0, 0)\n"
	if got != want {
		t.Errorf("Format(\"sphere.at(2,0,0)\")\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatSmoothOperator(t *testing.T) {
	got, err := Format("sphere|~0.3 box")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "sphere |~0.3 box\n"
	if got != want {
		t.Errorf("Format(\"sphere|~0.3 box\")\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatIdempotent(t *testing.T) {
	input := "sphere|box"
	first, err := Format(input)
	if err != nil {
		t.Fatalf("unexpected error on first format: %v", err)
	}
	second, err := Format(first)
	if err != nil {
		t.Fatalf("unexpected error on second format: %v", err)
	}
	if first != second {
		t.Errorf("not idempotent:\nfirst:  %q\nsecond: %q", first, second)
	}
}

func TestFormatMultiLine(t *testing.T) {
	got, err := Format("sphere\nbox.at(2,0,0)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "sphere | box.at(2, 0, 0)\n"
	if got != want {
		t.Errorf("Format multi-line\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatTrailingNewline(t *testing.T) {
	got, err := Format("sphere")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "sphere\n"
	if got != want {
		t.Errorf("Format(\"sphere\")\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatAssignment(t *testing.T) {
	got, err := Format("r = 1.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "r = 1.5\n"
	if got != want {
		t.Errorf("Format(\"r = 1.5\")\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatFunctionDef(t *testing.T) {
	got, err := Format("f(x) = sphere(x)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "f(x) = sphere(x)\n"
	if got != want {
		t.Errorf("Format function def\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatVecLit(t *testing.T) {
	got, err := Format("[1,2,3]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "[1, 2, 3]\n"
	if got != want {
		t.Errorf("Format vec lit\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatUnaryNeg(t *testing.T) {
	got, err := Format("-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "-1\n"
	if got != want {
		t.Errorf("Format unary neg\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatIdempotentComplex(t *testing.T) {
	input := "sphere.at(2, 0, 0).red | box.blue"
	first, err := Format(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := Format(first)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first != second {
		t.Errorf("not idempotent:\nfirst:  %q\nsecond: %q", first, second)
	}
}

func TestFormatParseError(t *testing.T) {
	_, err := Format("((((")
	if err == nil {
		t.Errorf("expected error for unparseable input")
	}
}

func TestFormatNamedArgs(t *testing.T) {
	got, err := Format("fbm(p,octaves: 6)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "fbm(p, octaves: 6)\n"
	if got != want {
		t.Errorf("Format named args\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatBareMethod(t *testing.T) {
	got, err := Format("sphere.red")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "sphere.red\n"
	if got != want {
		t.Errorf("Format bare method\ngot:  %q\nwant: %q", got, want)
	}
}
