; ── Comments ────────────────────────────────────────────────
(comment) @comment

; ── Literals ────────────────────────────────────────────────
(number) @number
(hex_color) @constant
(string) @string
(boolean) @constant.builtin

; ── Keywords ────────────────────────────────────────────────
["for" "in" "if" "else" "step"] @keyword
["light" "camera" "bg" "raymarch" "post" "mat" "debug" "glsl"] @keyword

; ── Built-in shapes ─────────────────────────────────────────
; 3D primitives
((identifier) @function.builtin
  (#match? @function.builtin "^(sphere|box|cylinder|torus|capsule|cone|plane|octahedron|pyramid|ellipsoid|rounded_box|wireframe_box|rounded_cylinder|capped_cylinder|capped_cone|rounded_cone)$"))

; 2D primitives
((identifier) @function.builtin
  (#match? @function.builtin "^(circle|rect|hexagon|polygon)$"))

; ── Built-in math / utility functions ───────────────────────
((identifier) @function.builtin
  (#match? @function.builtin "^(sin|cos|tan|asin|acos|atan|atan2|pow|sqrt|exp|log|floor|ceil|round|fract|abs|sign|min|max|mix|smoothstep|step|clamp|length|normalize|dot|cross|distance|reflect|noise|fbm|voronoi|warp|rgb|hsl|hsla|rgba|ease_in|ease_out|ease_in_out|ease_cubic_in|ease_cubic_out|ease_cubic_in_out|ease_elastic|ease_bounce|ease_back|ease_expo|pulse|saw|tri|remap|saturate|spring|keyframes)$"))

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

; ── Axis / direction constants ──────────────────────────────
((identifier) @constant.builtin
  (#match? @constant.builtin "^(up|down|left|right|forward|back)$"))

; ── Built-in variables ──────────────────────────────────────
((identifier) @variable.builtin
  (#match? @variable.builtin "^(t|p)$"))

; ── CSG operators ───────────────────────────────────────────
["|" "&"] @operator
["|~" "-~" "&~" "|/" "-/" "&/"] @operator

; ── Arithmetic / comparison operators ───────────────────────
["+" "-" "*" "/" "%" "!"] @operator
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
