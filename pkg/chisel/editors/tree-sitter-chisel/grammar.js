/// <reference types="tree-sitter-cli/dsl" />
// @ts-check
//
// Generated from pkg/chisel/lang/lang.go
// Do not edit — run: go generate ./pkg/chisel/lang/

module.exports = grammar({
  name: 'chisel',

  extras: $ => [/\s/, $.comment],

  word: $ => $.identifier,

  conflicts: $ => [
    [$.settings_block, $.block],
    [$.settings_entry, $.primary_expression],
    [$.param, $.primary_expression],
  ],

  rules: {
    program: $ => repeat(choice(
      $.setting,
      $.assignment,
      $.expression,
    )),

    // ── Comments ──────────────────────────────────────────────
    comment: $ => choice(
      seq('//', /[^\n]*/),
      seq('/*', /[^*]*\*+([^/*][^*]*\*+)*/, '/'),
    ),

    // ── Settings blocks ──────────────────────────────────────
    setting: $ => choice(
      seq('light', choice($.expression, $.settings_block)),
      seq('camera', choice($.camera_shorthand, $.settings_block)),
      seq('bg', choice($.expression, $.settings_block)),
      seq('raymarch', $.settings_block),
      seq('post', $.settings_block),
      seq('debug', $.identifier),
      seq('mat', $.identifier, '=', $.settings_block),
    ),

    camera_shorthand: $ => seq($.expression, '->', $.expression),

    settings_block: $ => seq('{', commaSep($.settings_entry), '}'),

    settings_entry: $ => choice(
      seq($.identifier, ':', $.expression),
      seq($.identifier, $.settings_block),
    ),

    // ── Assignments ──────────────────────────────────────────
    assignment: $ => seq(
      $.identifier,
      optional($.params),
      '=',
      $.expression,
    ),

    params: $ => seq('(', commaSep($.param), ')'),

    param: $ => seq($.identifier, optional(seq('=', $.expression))),

    // ── Expressions (with precedence) ────────────────────────
    expression: $ => choice(
      $.binary_expression,
      $.unary_expression,
      $.method_chain,
      $.primary_expression,
    ),

    binary_expression: $ => choice(
      prec.left(0, seq($.expression, '..', $.expression)),
      prec.left(1, seq($.expression, '|', $.expression)),
      prec.left(1, seq($.expression, '|~', optional($.expression), $.expression)),
      prec.left(1, seq($.expression, '|/', optional($.expression), $.expression)),
      prec.left(2, seq($.expression, '-', $.expression)),
      prec.left(2, seq($.expression, '-~', optional($.expression), $.expression)),
      prec.left(2, seq($.expression, '-/', optional($.expression), $.expression)),
      prec.left(3, seq($.expression, '&', $.expression)),
      prec.left(3, seq($.expression, '&~', optional($.expression), $.expression)),
      prec.left(3, seq($.expression, '&/', optional($.expression), $.expression)),
      prec.left(4, seq($.expression, '==', $.expression)),
      prec.left(4, seq($.expression, '!=', $.expression)),
      prec.left(4, seq($.expression, '<', $.expression)),
      prec.left(4, seq($.expression, '>', $.expression)),
      prec.left(4, seq($.expression, '<=', $.expression)),
      prec.left(4, seq($.expression, '>=', $.expression)),
      prec.left(5, seq($.expression, '+', $.expression)),
      prec.left(6, seq($.expression, '*', $.expression)),
      prec.left(6, seq($.expression, '/', $.expression)),
      prec.left(6, seq($.expression, '%', $.expression)),
    ),

    unary_expression: $ => choice(
      prec(7, seq('-', $.expression)),
      prec(7, seq('!', $.expression)),
    ),

    method_chain: $ => prec.left(8, seq(
      $.primary_expression,
      repeat1(seq('.', choice($.method_call, $.swizzle))),
    )),

    method_call: $ => choice(
      prec(1, seq($.identifier, '(', commaSep($.argument), ')')),
      $.identifier,
    ),

    swizzle: $ => /[xyzrgb]{1,4}/,

    argument: $ => choice(
      seq($.identifier, ':', $.expression),   // named argument
      $.expression,                            // positional argument
    ),

    // ── Primary expressions ──────────────────────────────────
    primary_expression: $ => choice(
      $.number,
      $.boolean,
      $.string,
      $.hex_color,
      $.vector,
      $.function_call,
      $.identifier,
      seq('(', $.expression, ')'),
      $.block,
      $.for_expression,
      $.if_expression,
      $.glsl_escape,
    ),

    function_call: $ => prec(1, seq(
      $.identifier,
      '(',
      commaSep($.argument),
      ')',
    )),

    block: $ => seq('{', repeat(choice($.assignment, $.expression)), '}'),

    vector: $ => seq('[', commaSep1($.expression), ']'),

    // ── Control flow ─────────────────────────────────────────
    for_expression: $ => seq(
      'for',
      commaSep1($.iterator),
      $.block,
    ),

    iterator: $ => seq(
      $.identifier,
      'in',
      $.expression,
      '..',
      $.expression,
      optional(seq('step', $.expression)),
    ),

    if_expression: $ => seq(
      'if',
      $.expression,
      $.block,
      optional(seq('else', choice($.if_expression, $.block))),
    ),

    // ── GLSL escape ──────────────────────────────────────────
    glsl_escape: $ => seq(
      'glsl',
      '(',
      $.identifier,
      ')',
      $.glsl_body,
    ),

    glsl_body: $ => seq('{', /([^{}]|\{([^{}]|\{[^{}]*\})*\})*/, '}'),

    // ── Terminals ────────────────────────────────────────────
    number: $ => {
      const decimal_digits = /\d+/;
      const decimal_point = /\.\d+/;
      const exponent = /[eE][+-]?\d+/;
      return token(choice(
        seq(decimal_digits, decimal_point, optional(exponent)),  // 1.5, 1.5e10
        seq(decimal_digits, exponent),                           // 1e10
        decimal_digits,                                          // 42
      ));
    },

    boolean: $ => choice('true', 'false'),

    string: $ => choice(
      seq('"', /[^"]*/, '"'),
      seq("'", /[^']*/, "'"),
    ),

    hex_color: $ => token(seq('#', /[0-9a-fA-F]{3,8}/)),

    identifier: $ => /[a-zA-Z_][a-zA-Z0-9_]*/,
  },
});

/**
 * Comma-separated list (zero or more).
 */
function commaSep(rule) {
  return optional(commaSep1(rule));
}

/**
 * Comma-separated list (one or more).
 */
function commaSep1(rule) {
  return seq(rule, repeat(seq(',', rule)));
}

