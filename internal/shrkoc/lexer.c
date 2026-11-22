/* shrkc/lexer.c */
#include "lexer.h"
#include <ctype.h>
#include <string.h>

void shr_lexer_init(ShrLexer *lx, const char *src, size_t len) {
    lx->src = src;
    lx->pos = 0;
    lx->len = len;
    lx->line = 1;
}

static bool is_ident_start(char c) {
    return isalpha((unsigned char)c) || c == '_';
}

static bool is_ident_char(char c) {
    return isalnum((unsigned char)c) || c == '_';
}

static ShrTokenKind kw_or_ident(const char *s, size_t len) {
    if (len == 2 && memcmp(s, "fn", 2) == 0)     return TOK_KW_FN;
    if (len == 3 && memcmp(s, "let", 3) == 0)    return TOK_KW_LET;
    if (len == 6 && memcmp(s, "return", 6) == 0) return TOK_KW_RETURN;
    if (len == 3 && memcmp(s, "use", 3) == 0)    return TOK_KW_USE;
    if (len == 3 && memcmp(s, "for", 3) == 0)    return TOK_KW_FOR;
    if (len == 4 && memcmp(s, "true", 4) == 0)   return TOK_KW_TRUE;
    if (len == 5 && memcmp(s, "false", 5) == 0)  return TOK_KW_FALSE;
    return TOK_IDENT;
}

ShrToken shr_lexer_next(ShrLexer *lx) {
    while (lx->pos < lx->len) {
        char c = lx->src[lx->pos];
        if (c == '\n') { lx->line++; lx->pos++; continue; }
        if (isspace((unsigned char)c)) { lx->pos++; continue; }
        if (c == '/' && lx->pos + 1 < lx->len && lx->src[lx->pos + 1] == '/') {
            while (lx->pos < lx->len && lx->src[lx->pos] != '\n') lx->pos++;
            continue;
        }
        break;
    }

    if (lx->pos >= lx->len) {
        return (ShrToken){ .kind = TOK_EOF, .start = lx->src + lx->len, .len = 0, .line = lx->line };
    }

    int line = lx->line;
    const char *start = lx->src + lx->pos;
    char c = lx->src[lx->pos++];

    if (is_ident_start(c)) {
        while (lx->pos < lx->len && is_ident_char(lx->src[lx->pos])) lx->pos++;
        size_t n = (size_t)((lx->src + lx->pos) - start);
        return (ShrToken){ .kind = kw_or_ident(start, n), .start = start, .len = n, .line = line };
    }

    if (isdigit((unsigned char)c)) {
        bool is_flt = false;
        while (lx->pos < lx->len && isdigit((unsigned char)lx->src[lx->pos])) lx->pos++;
        if (lx->pos < lx->len && lx->src[lx->pos] == '.' &&
            lx->pos + 1 < lx->len && isdigit((unsigned char)lx->src[lx->pos + 1])) {
            is_flt = true;
            lx->pos++;
            while (lx->pos < lx->len && isdigit((unsigned char)lx->src[lx->pos])) lx->pos++;
        }
        return (ShrToken){
            .kind = is_flt ? TOK_FLOAT : TOK_INT,
            .start = start,
            .len = (size_t)((lx->src + lx->pos) - start),
            .line = line
        };
    }

    if (c == '"') {
        while (lx->pos < lx->len && lx->src[lx->pos] != '"') {
            if (lx->src[lx->pos] == '\\' && lx->pos + 1 < lx->len) lx->pos++;
            if (lx->src[lx->pos] == '\n') lx->line++;
            lx->pos++;
        }
        if (lx->pos < lx->len && lx->src[lx->pos] == '"') lx->pos++;
        return (ShrToken){
            .kind = TOK_STRING,
            .start = start,
            .len = (size_t)((lx->src + lx->pos) - start),
            .line = line
        };
    }

    if (c == '-' && lx->pos < lx->len && lx->src[lx->pos] == '>') {
        lx->pos++;
        return (ShrToken){ .kind = TOK_ARROW, .start = start, .len = 2, .line = line };
    }
    if (c == '-' && lx->pos < lx->len && lx->src[lx->pos] == '=') {
        lx->pos++;
        return (ShrToken){ .kind = TOK_MINUS_EQ, .start = start, .len = 2, .line = line };
    }
    if (c == '-' && lx->pos < lx->len && lx->src[lx->pos] == '-') {
        lx->pos++;
        return (ShrToken){ .kind = TOK_MINUS_MINUS, .start = start, .len = 2, .line = line };
    }
    if (c == '+' && lx->pos < lx->len && lx->src[lx->pos] == '=') {
        lx->pos++;
        return (ShrToken){ .kind = TOK_PLUS_EQ, .start = start, .len = 2, .line = line };
    }
    if (c == '+' && lx->pos < lx->len && lx->src[lx->pos] == '+') {
        lx->pos++;
        return (ShrToken){ .kind = TOK_PLUS_PLUS, .start = start, .len = 2, .line = line };
    }
    if (c == '*' && lx->pos < lx->len && lx->src[lx->pos] == '=') {
        lx->pos++;
        return (ShrToken){ .kind = TOK_STAR_EQ, .start = start, .len = 2, .line = line };
    }
    if (c == '/' && lx->pos < lx->len && lx->src[lx->pos] == '=') {
        lx->pos++;
        return (ShrToken){ .kind = TOK_SLASH_EQ, .start = start, .len = 2, .line = line };
    }
    if (c == '=' && lx->pos < lx->len && lx->src[lx->pos] == '=') {
        lx->pos++;
        return (ShrToken){ .kind = TOK_EQ_EQ, .start = start, .len = 2, .line = line };
    }
    if (c == '!' && lx->pos < lx->len && lx->src[lx->pos] == '=') {
        lx->pos++;
        return (ShrToken){ .kind = TOK_BANG_EQ, .start = start, .len = 2, .line = line };
    }
    if (c == '<' && lx->pos < lx->len && lx->src[lx->pos] == '=') {
        lx->pos++;
        return (ShrToken){ .kind = TOK_LE, .start = start, .len = 2, .line = line };
    }
    if (c == '>' && lx->pos < lx->len && lx->src[lx->pos] == '=') {
        lx->pos++;
        return (ShrToken){ .kind = TOK_GE, .start = start, .len = 2, .line = line };
    }

    switch (c) {
        case '(': return (ShrToken){ .kind = TOK_LPAREN,   .start = start, .len = 1, .line = line };
        case ')': return (ShrToken){ .kind = TOK_RPAREN,   .start = start, .len = 1, .line = line };
        case '{': return (ShrToken){ .kind = TOK_LBRACE,   .start = start, .len = 1, .line = line };
        case '}': return (ShrToken){ .kind = TOK_RBRACE,   .start = start, .len = 1, .line = line };
        case '[': return (ShrToken){ .kind = TOK_LBRACKET, .start = start, .len = 1, .line = line };
        case ']': return (ShrToken){ .kind = TOK_RBRACKET, .start = start, .len = 1, .line = line };
        case ',': return (ShrToken){ .kind = TOK_COMMA,    .start = start, .len = 1, .line = line };
        case ':': return (ShrToken){ .kind = TOK_COLON,    .start = start, .len = 1, .line = line };
        case '=': return (ShrToken){ .kind = TOK_EQ,       .start = start, .len = 1, .line = line };
        case '<': return (ShrToken){ .kind = TOK_LT,       .start = start, .len = 1, .line = line };
        case '>': return (ShrToken){ .kind = TOK_GT,       .start = start, .len = 1, .line = line };
        case ';': return (ShrToken){ .kind = TOK_SEMI,     .start = start, .len = 1, .line = line };
        case '.': return (ShrToken){ .kind = TOK_DOT,      .start = start, .len = 1, .line = line };
        case '+': return (ShrToken){ .kind = TOK_PLUS,     .start = start, .len = 1, .line = line };
        case '-': return (ShrToken){ .kind = TOK_MINUS,    .start = start, .len = 1, .line = line };
        case '*': return (ShrToken){ .kind = TOK_STAR,     .start = start, .len = 1, .line = line };
        case '/': return (ShrToken){ .kind = TOK_SLASH,    .start = start, .len = 1, .line = line };
        case '%': return (ShrToken){ .kind = TOK_PERCENT,  .start = start, .len = 1, .line = line };
        default:  return (ShrToken){ .kind = TOK_EOF,      .start = start, .len = 1, .line = line };
    }
}
