(block "{" @indent)
(block "}" @outdent)

(settings_block "{" @indent)
(settings_block "}" @outdent)

(for_expression (block "{" @indent))
(for_expression (block "}" @outdent))

(if_expression (block "{" @indent))
(if_expression (block "}" @outdent))

(glsl_body "{" @indent)
(glsl_body "}" @outdent)
