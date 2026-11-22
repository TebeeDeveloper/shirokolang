/* shrkc/lexer.h */
#pragma once

#include <stddef.h>
#include <stdbool.h>

typedef enum {
    TOK_EOF = 0,
    TOK_IDENT,
    TOK_INT,
    TOK_FLOAT,
    TOK_STRING,
    TOK_KW_FN,
    TOK_KW_LET,
    TOK_KW_RETURN,
    TOK_KW_USE,
    TOK_KW_FOR,
    TOK_KW_TRUE,
    TOK_KW_FALSE,
    TOK_KW_IF,
    TOK_KW_ELSE,
    TOK_LPAREN,     /* ( */
    TOK_RPAREN,     /* ) */
    TOK_LBRACE,     /* { */
    TOK_RBRACE,     /* } */
    TOK_LBRACKET,   /* [ */
    TOK_RBRACKET,   /* ] */
    TOK_COMMA,      /* , */
    TOK_COLON,      /* : */
    TOK_ARROW,      /* -> */
    TOK_EQ,         /* = */
    TOK_PLUS_EQ,    /* += */
    TOK_MINUS_EQ,   /* -= */
    TOK_STAR_EQ,    /* *= */
    TOK_SLASH_EQ,   /* /= */
    TOK_PLUS_PLUS,  /* ++ */
    TOK_MINUS_MINUS,/* -- */
    TOK_EQ_EQ,      /* == */
    TOK_BANG_EQ,    /* != */
    TOK_LT,         /* < */
    TOK_LE,         /* <= */
    TOK_GT,         /* > */
    TOK_GE,         /* >= */
    TOK_SEMI,       /* ; */
    TOK_DOT,        /* . */
    TOK_PLUS,
    TOK_MINUS,
    TOK_STAR,
    TOK_SLASH,
    TOK_PERCENT,
    TOK_AND_AND,
    TOK_OR_OR,
    TOK_BANG,
} ShrTokenKind;

typedef struct {
    ShrTokenKind kind;
    const char *start;
    size_t len;
    int line;
} ShrToken;

typedef struct {
    const char *src;
    size_t pos;
    size_t len;
    int line;
} ShrLexer;

void shr_lexer_init(ShrLexer *lx, const char *src, size_t len);
ShrToken shr_lexer_next(ShrLexer *lx);
