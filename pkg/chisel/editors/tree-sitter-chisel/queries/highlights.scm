; ── Generated from pkg/chisel/lang/lang.go ──────────────────
; Do not edit — run: go generate ./pkg/chisel/lang/

; ── Comments ────────────────────────────────────────────────
(comment) @comment

; ── Literals ────────────────────────────────────────────────
(number) @number
(hex_color) @constant
(string) @string
(boolean) @constant.builtin

; ── Keywords ────────────────────────────────────────────────
["for" "in" "if" "else" "step" "glsl"] @keyword
["light" "camera" "bg" "raymarch" "post" "debug" "mat"] @keyword

; ── Built-in shapes ─────────────────────────────────────────
((identifier) @function.builtin
  (#match? @function.builtin "^(sphere|box|cylinder|torus|capsule|cone|plane|octahedron|pyramid|ellipsoid|rounded_box|box_frame|capped_torus|hex_prism|octagon_prism|round_cone|tri_prism|capped_cone|solid_angle|rhombus|horseshoe|rounded_cylinder|tetrahedron|dodecahedron|icosahedron|slab)$"))

((identifier) @function.builtin
  (#match? @function.builtin "^(circle|rect|hexagon|polygon|triangle|egg)$"))

; ── Built-in functions ─────────────────────────────────────
((identifier) @function.builtin
  (#match? @function.builtin "^(sin|cos|tan|asin|acos|atan|atan2|pow|sqrt|exp|log|floor|ceil|round|fract|abs|sign|min|max|mix|smoothstep|step|clamp|saturate|remap|length|normalize|dot|cross|distance|reflect|mod|radians|degrees|noise|fbm|voronoi|pulse|saw|tri|ease_in|ease_out|ease_in_out|ease_cubic_in|ease_cubic_out|ease_cubic_in_out|ease_elastic|ease_bounce|ease_back|ease_expo|rgb|hsl|hsla|rgba)$"))

; ── Method calls ────────────────────────────────────────────
(method_call (identifier) @method)

; ── Swizzle ─────────────────────────────────────────────────
(swizzle) @property

; ── Named colors ────────────────────────────────────────────
((identifier) @constant.builtin
  (#match? @constant.builtin "^(red|green|blue|white|black|yellow|cyan|magenta|gray|orange|purple|pink)$"))

; ── Constants ───────────────────────────────────────────────
((identifier) @constant.builtin
  (#match? @constant.builtin "^(PI|TAU|E)$"))

; ── Built-in variables ──────────────────────────────────────
((identifier) @variable.builtin
  (#match? @variable.builtin "^(t|p)$"))

; ── CSG operators ───────────────────────────────────────────
["|" "&"] @operator
["|~" "|/" "|@" "|!" "|^" "-~" "-/" "&~" "&/"] @operator

; ── Arithmetic / comparison operators ───────────────────────
["-" "+" "*" "/" "%" "!"] @operator
["==" "!=" "<" ">" "<=" ">="] @operator
["="] @operator

; ── Punctuation ─────────────────────────────────────────────
["(" ")" "[" "]" "{" "}"] @punctuation.bracket
["." "," ":" ".." "->"] @punctuation.delimiter

; ── Assignment target ───────────────────────────────────────
(assignment (identifier) @variable)

; ── Function definition (assignment with params) ────────────
(assignment (identifier) @function (params))

; ── Parameter names ─────────────────────────────────────────
(param (identifier) @variable.parameter)

; ── Settings keys ───────────────────────────────────────────
(settings_entry (identifier) @property)
