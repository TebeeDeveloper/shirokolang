---
title: "Báo cáo kỹ thuật: Ngôn ngữ lập trình ShirokoLang"
author: "Trần Trung Nghĩa, Trần Đào Gia Minh"
date: "2026"
---

# ShirokoLang
## Báo cáo kỹ thuật: Ngôn ngữ lập trình ShirokoLang

- Tác giả:
  - Trần Trung Nghĩa (technical leader) — lớp 10A6
  - Trần Đào Gia Minh (web designer) — lớp 12A1
- Đơn vị: trường THPT Nguyễn Thần Hiến
- Phiên bản: 26.10 (năm 2026)

## 1. Tổng quan (Overview)
### 1.1. Giới thiệu

- ShirokoLang là một ngôn ngữ lập trình biên dịch được thiết kế với mục tiêu 
**dễ học, dễ sử dụng** cho lứa tuổi học sinh và nhà nghiên cứu kỹ thuật.
- Triết lý thiết kế cốt lõi có thể tóm gọn trong một từ: **"Compact" (nhỏ gọn)** — 
**nhỏ gọn, nhanh nhạy, linh hoạt**. Đây cũng là tinh thần xuyên suốt từ cú pháp,
hệ thống kiểu, đến công cụ kiểm thử shirocc (Shiroko Compact Compiler).
- Ngôn ngữ được phát triển như một dự án học tập về nguyên lý ngôn ngữ lập trình 
và trình biên dịch (compiler).

### 1.2 Động lực
- Việt Nam hiện nay chưa có ngôn ngữ lập trình thân thiện 
với người học và mục tiêu nghiên cứu. Mặc dù Python hay C++ 
đã đáp ứng phần nào nhu cầu hiện có, nhưng cú pháp của chúng 
khá phức tạp và không hợp với lối đọc hiểu ngắn gọn, hàm súc của
người học. Python dùng range half-open [a,b) gây nhầm lẫn cho người mới; ShirokoLang chọn closed range [a,b] nhất quán
- Ngôn ngữ lập trình thân thiện là một vấn đề chưa được giải quyết tốt đối với ngành giáo dục và 
các ngành nghiên cứu hiện tại.

### 1.3 Đối tượng hướng đến
- Học sinh
- Người mới học lập trình
- Lập trình viên muốn thử ngôn ngữ mới
- Nhà nghiên cứu cần một ngôn ngữ đơn giản

## 2. Đặc điểm ngôn ngữ (Features)
- Kiểu tĩnh
- Lập trình hướng đối tượng
- Lập trình hàm

### 2.1 Cú pháp cơ bản
#### Khai báo biến
```
let x = 10
const PI = 3.14
```
#### Hàm
```
fn cong(a, b int) int { // lưu ý, Shiroko hỗ trợ nhận kiểu liền kề
  return a + b
}
```
#### Giao diện và cấu trúc
```
interface GiaoDien {
  area() float
}

struct HinhVuong {
  dai int
}

fn (HinhVuong) area() float {
  return self.dai * self.dai
}
```
#### Vòng lặp và điều kiện
```
if x > 0 {}

for i = 0, i < 10, i++ {}
for range 10 {}

let ds = []int{1, 2, 3}

for d = iter ds {}
for i, d = iter ds {}

let x = 7

match x {
  10 => "ten"
  5..9 => "range 5..9"
  _ => "little"
}
```
#### Danh sách
```
let danhSach = []int{1, 2, 3}
```

#### Shiroprehension (list comprehension)
```
// lọc
let comp = []int{x @ danhSach | x % 2 == 0; x < 30}

// lấy phần theo điều kiện biên
let cmp = []int{x @ danhSach; x > 2}
````
#### Dùng module:
```
import {
  "std/fmt"
}
```
#### Ép kiểu:
```
const PI = 3.14
let x = int(PI)
```
#### ví dụ chương trình Hello World:
```
package main

import {
  "std/fmt"
}
fn main() {
  fmt.println("Hello, ShirokoLang!")
}
```

### 2.2 Bảng toán tử và ký hiệu

|Ký hiệu|Ý nghĩa|Ví dụ|
|---|---|---|
|`=`|gán|`let x = 10`|
| \| | filter trong (inner) — kiểm tra sau `;` | `[]int{x @ ds \| x % 2 == 0}` |
| `;` | filter ngoài (outer) — kiểm tra trước \| | `[]int{x @ ds; x < 30}` |
|`,`|phân tách tham số / phần tử|`fmt.println(1, 2, 3)`|
|`-`, `!`|âm, phủ định|`-a`, `!điều_kiện`|
|`true`, `false`|literal bool|`let x = false`|
|\|\|, `&&`|hoặc, và	|`if a > 0 \|\| b > 0 {}`|
|`+`, `-,` `*`, `/`, `%`|số học|`a + b`|
|`==`, `!=`, `<`, `>`, `<=`, `>=`|so sánh|`x == 10`|
|`..`|khoảng|`1..10`, `5..9`|
|`=>`|match arm|`10 => "ten"`|
| `@` | "thuộc" — nguồn trong Shiroprehension (∈) | `x @ danhSach` |
|`++`|tăng	|`i++`|


### 2.3 EBNF

- Quy ước ký hiệu EBNF dùng trong tài liệu này

|Ký hiệu|Ý nghĩa|
|---|---|
|`=`|định nghĩa rule|
|`\|`|hoặc|
|`,`|nối tiếp|
|`{ ... }`|lặp 0 hoặc nhiều lần|
|`[ ... ]`|tùy chọn (0 hoặc 1)|
|`( ... )`|nhóm|
|`"..."`|terminal (literal)|
|`(* ... *)`|comment|

#### a. Lexical grammar (token)
```
(* ===== Ký tự cơ bản ===== *)
letter       = "a".."z" | "A".."Z" | "_" ;
digit        = "0".."9" ;
alnum        = letter | digit ;

(* ===== Định danh ===== *)
identifier   = letter , { alnum } ;

(* ===== Literal ===== *)
number       = digit , { digit } , [ "." , digit , { digit } ] ;
string       = '"' , { stringChar } , '"' ;
stringChar   = anyChar - '"' - "\" - "\n"
             | "\" , escapeChar ;
escapeChar   = "n" | "t" | "r" | '"' | "\" | "0" ;
bool         = "true" | "false" ;

(* ===== Từ khóa ===== *)
keyword      = "package" | "import" | "let" | "const" | "fn"
             | "struct" | "interface" | "if" | "else"
             | "for" | "range" | "iter" | "match"
             | "return" | "self" | "true" | "false"
             | "int" | "float" | "string" | "bool" | "byte" | "any"
             | "map" ;

(* ===== Toán tử ===== *)
opArith      = "+" | "-" | "*" | "/" | "%" ;
opCompare    = "==" | "!=" | "<" | ">" | "<=" | ">=" ;
opLogic      = "||" | "&&" ;
opUnary      = "-" | "!" ;
opAssign     = "=" ;
opRange      = ".." ;
opArrow      = "=>" ;
opFilter     = "|" ;
opAt         = "@" ;
opInc        = "++" ;
opDec        = "--" ;

(* ===== Dấu câu ===== *)
lparen       = "(" ;   rparen = ")" ;
lbrace       = "{" ;   rbrace = "}" ;
lbracket     = "[" ;   rbracket = "]" ;
comma        = "," ;   dot    = "." ;

(* ===== Comment ===== *)
comment      = "//" , { anyChar - "\n" } ;
```

#### b. Syntax grammar (cú pháp)
```text
(* ===== Chương trình ===== *)
program       = packageClause , { importClause } , { topDecl } ;
packageClause = "package" , identifier ;
importClause  = "import" , "{" , { string } , "}" ;

topDecl       = varDecl | constDecl | function | method
              | structDecl | interfaceDecl ;

(* ===== Khai báo biến / hằng ===== *)
varDecl       = "let" , identifier , [ type ] , "=" , expr ;
constDecl     = "const" , identifier , [ type ] , "=" , expr ;

(* ===== Hàm ===== *)
function      = "fn" , identifier , "(" , [ params ] , ")" ,
                [ type ] , block ;
method        = "fn" , "(" , receiverType , ")" , methodName ,
                "(" , [ params ] , ")" , [ type ] , block ;
receiverType  = identifier ;
methodName    = identifier ;

params        = paramGroup , { "," , paramGroup } ;
paramGroup    = identifier , { "," , identifier } , type ;

(* ===== Struct / Interface ===== *)
structDecl    = "struct" , identifier , "{" , { field } , "}" ;
field         = identifier , type ;

interfaceDecl = "interface" , identifier , "{" , { methodSig } , "}" ;
methodSig     = identifier , "(" , [ params ] , ")" , [ type ] ;

(* ===== Khối lệnh ===== *)
block         = "{" , { statement } , "}" ;
statement     = varDecl | constDecl | ifStmt
              | forRange | forClassic | forIter
              | matchStmt | returnStmt | exprStmt ;

exprStmt      = expr ;

(* ===== Điều kiện ===== *)
ifStmt        = "if" , expr , block , [ "else" , ( ifStmt | block ) ] ;

(* ===== Vòng lặp ===== *)
forRange        = "for" , "range" , expr , block ;
forClassic      = "for" , identifier , "=" , expr , "," , expr , "," ,
                  step , block ;
forIter         = "for" , [ identifier , "," ] , identifier ,
                  "=" , "iter" , expr , block ;

step            = identifier , ( "++" | "--" )
                | identifier , "=" , expr ;

(* ===== Match ===== *)
matchStmt     = "match" , expr , "{" , { matchArm } , "}" ;
matchArm      = pattern , "=>" , expr , [ "," ] ;
pattern       = literal | rangePattern | "_" ;
rangePattern  = expr , ".." , expr ;
literal       = number | string | bool ;

(* ===== Return ===== *)
returnStmt    = "return" , [ expr ] ;

(* ===== Biểu thức (theo độ ưu tiên) ===== *)
expr          = logicOr ;
logicOr       = logicAnd , { "||" , logicAnd } ;
logicAnd      = equality , { "&&" , equality } ;
equality      = comparison , { ( "==" | "!=" ) , comparison } ;
comparison    = term , { ( "<" | ">" | "<=" | ">=" ) , term } ;
term          = factor , { ( "+" | "-" ) , factor } ;
factor        = unary , { ( "*" | "/" | "%" ) , unary } ;
unary         = { "-" | "!" } , postfix ;
postfix       = primary , { "(" , [ args ] , ")"
                          | "[" , expr , "]"
                          | dot , identifier
                          | "++"
                          | "--" } ;

primary       = literal
              | identifier
              | "(" , expr , ")"
              | cast
              | lambdaExpr
              | listLit
              | compLit ;

lambdaExpr    = "fn" , "(" , [ params ] , ")" , [ type ] , block ;

(* ===== CAST ===== *)
(* Cú pháp giống function call, ngữ nghĩa là ép kiểu *)
cast          = type , "(" , expr , ")" ;

(* ===== List ===== *)
listLit       = "[]" , type , "{" , [ exprList ] , "}" ;
exprList      = expr , { "," , expr } ;

(* ===== Shiroprehension ===== *)
compLit = "[]" , type , "{" , expr , "@" , expr ,
          [ "|" , expr ] , [ ";" , expr ] , "}" ;
(* Mô phỏng ký hiệu tập hợp: { x ∈ S | P(x) ; Q(x) }
   `|` mở đầu tính chất, `;` nối điều kiện tiếp theo.
   Cả hai là AND. `;` chạy trước `|` chỉ là tối ưu thực thi. *)

(* ===== Kiểu ===== *)
type          = primitiveType | listType | mapType ;
primitiveType = "int" | "float" | "string" | "bool" | "byte" | "any" ;
listType      = "[]" , type ;
mapType       = "map" , "[" , type , "]" , type ;

(* ===== Tham số cho lời gọi hàm ===== *)
args          = expr , { "," , expr } ;
```

### 2.4 Thuật ngữ (Glossary)

| Thuật ngữ | Ký hiệu / Cú pháp | Nghĩa |
|---|---|---|
| Shiroprehension | `[]T{...}` | Cú pháp tạo mảng, mô phỏng ký hiệu tập hợp |
| thuộc | `@` | Nguồn trong Shiroprehension (∈) |
| sao cho | `\|` | Mở đầu điều kiện lọc |
| và | `;` | Nối điều kiện lọc |
| shirocc | — | Shiroko Compact Compiler |
| self | — | Tham chiếu receiver trong method |
| range đóng | `a..b` | `[a,b]`, bao gồm cả 2 đầu |

### 2.5 Quy tắc ngữ nghĩa

| Quy tắc | Mô tả |
|---|---|
| Range | `a..b` là closed `[a,b]`, áp dụng nhất quán |
| Match overlap | Arm giao nhau → lỗi compile |
| Shiroprehension | `;` (và) kiểm tra trước `\|` (sao cho) |
| Ép kiểu | widening tự động, narrowing tường minh |

## 3. Kiến trúc hệ thống (Architecture)
### 3.1 Sơ đồ tổng quát

```
┌─────────────────── shiroko/core ───────────────────┐
│                                                    │
│  .shrko → Lexer → Parser → AST → Semantic → Lower  │
│                                                    │
└────────────────────────┬───────────────────────────┘
                         │ IR
                         ▼
┌─────────────────── shiroko/backend ────────────────┐
│                                                    │
│   ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐   │
│   │ Go     │  │ Python │  │ C      │  │ ASM    │   │
│   └────────┘  └────────┘  └────────┘  └────────┘   │
│                                                    │
└────────────────────────────────────────────────────┘
```

### 3.2 Các thành phần
#### a. Lexer (bộ phân tích từ vựng)
- Chuyển chuỗi ký tự thành danh sách các token
- Xử lý: số, chuỗi, từ khóa, toán tử, comment
- Công cụ: viết tay

#### b. Parser (bộ phân tích cú pháp)
- Parser được viết tay hoàn toàn (hand-written), không sử dụng go/parser hay generator. 
Kết hợp Recursive Descent cho statement/declaration và Pratt Parser cho expression để xử lý độ ưu tiên toán tử gọn gàng
- Xây dựng AST từ Token
- Phương pháp: Lai tạo giữa Recursive Descent và Pratt Parser
- Xử lý độ ưu tiên toán tử

#### c. Semantic Checker (bộ phân tích ngữ nghĩa)

- Phân tích range coverage cho match arm: báo lỗi khi arm
  giao với arm trước (kể cả giao tại biên).
- Kiểm tra kiểu tĩnh, ép kiểu tự động/tường minh.
- Kiểm tra khai báo trùng lặp, chưa khai báo.
- Kiểm tra nhập các module.

#### d. Lower (bộ chuyển đổi AST)
- Lặp qua AST dịch sang IR

#### e. Backend
- Nơi chứa đựng thành phần gọi để dịch IR sang các ngôn ngữ đích: hợp ngữ, python, go, c, c++

| Backend | Trạng thái |
|---|---|
| Python | Thử nghiệm |
| Go | Hoàn chỉnh |
| C | Thử nghiệm |
| ASM | Thử nghiệm |

### 3.3 Công nghệ sử dụng
- Ngôn ngữ viết compiler: Go
- Thư viện thứ ba: không phụ thuộc bên thứ ba
- Thư viện Go: `fmt`, `strings`, `strconv`, `bytes`
- Build Tool: `gc`

### 3.4 Sử dụng
```bash
$ shirocc compile go main.shrko main.go
```

### Liên kết dự án

- GitHub: https://github.com/TebeeDeveloper/shirokolang

## 4. Hệ thống kiểu
- Các kiểu dữ liệu nguyên thủy: int, float, string, bool, byte, any
- Các kiểu dữ liệu mảng: []T
- Các kiểu dữ liệu từ điển: map[K]V
- Kiểm tra kiểu: tĩnh
- Cách ép kiểu: tự động lẫn tường minh

### 4.1 Generics (dự kiến v1.27)
- Hiện chưa hỗ trợ generic function/struct.
- Kiến trúc đã sẵn sàng — chỉ cần thêm vài dòng ở parser + semantic.
- Dự kiến: `fn f[T](x T) T`, `struct Box[T] { value T }`.

## 5. Ví dụ minh họa (Examples)
### 5.1 Tính giai thừa
```text
fn factorial(n int) int {
  if n <= 1 { return 1 }
  return n * factorial(n - 1)
}
```
### 5.2 Fibonacci
```text
fn fib(n int) int {
  if n < 2 { return n }
  return fib(n-1) + fib(n-2)
}
```
### 5.3 Ứng dụng thực tế
- Script tự động hóa
- Tính toán
- Chương trình ví dụ về lập trình đối tượng tách biệt
- Viết mã phục vụ nghiên cứu
- Sử dụng để thực hiện các cuộc thi lập trình hiện đại

## 6. Kiểm thử (Testing)

### 6.1. Danh sách test case

- **Số lượng test case:** 12 PASS + 1 SKIP
- **Công cụ test:** shirocc (Shiroko Compact Compiler)

| STT | File test | Nội dung kiểm tra | Kết quả |
|---|---|---|---|
| 1 | `main.shrko` | HelloWorld, import module `std/fmt` | PASS |
| 2 | `comp.shrko` | Shiroprehension: lọc `\|`, `;`, range literal | PASS |
| 3 | `lexer.shrko` | Token hóa: số, chuỗi, comment, toán tử; lexer self-hosting | PASS |
| 4 | `match.shrko` | Match + range `1..10`, chồng lấn `5..15`, bao trùm `1..20 + 5..15` | PASS |
| 5 | `var.shrko` | Khai báo `let`, `const`, ép kiểu tường minh `int(PI)` | PASS |
| 6 | `arith.shrko` | Toán tử số học, độ ưu tiên, kết hợp `+ - * / %` | PASS |
| 7 | `logic.shrko` | Toán tử logic `&&`, `\|\|`, `!`, so sánh `== != < > <= >=` | PASS |
| 8 | `func.shrko` | Hàm, đệ quy (factorial, fib), tham số nhiều kiểu | PASS |
| 9 | `struct.shrko` | `struct`, `interface`, method có receiver `self` | PASS |
| 10 | `loop.shrko` | `for range`, `for` cổ điển, `for iter` (có/không index) | PASS |
| 11 | `cast.shrko` | Ép kiểu `int(float)`, `float(int)`, `string(int)` | PASS |
| 12 | `lambda.shrko` | Lambda expression, truyền hàm như tham số | PASS |
| 13 | `generics.shrko` | Generics (dự kiến v1.27) | SKIP |

*Lưu ý: các file test nằm ở mục `tests/` của dự án*

### 6.2. Edge case đã xử lý

| Edge case | Mô tả | File test |
|---|---|---|
| Match chồng lấn | Nhiều arm cùng khớp một giá trị — báo lỗi chồng lấp | `match.shrko` |
| Match bao trùm | Range `1..20` và `5..15` — báo lỗi chồng lấp | `match.shrko` |
| Overlap tại biên | `0..5` và `5..6.5` giao tại 5.0 — báo lỗi chồng lấp| `match.shrko` |
| Match mặc định | `_` bắt mọi giá trị còn lại | `match.shrko` |
| Range closed | `1..10` bao gồm 10, khác Python | `match.shrko` |
| Shiroprehension `\|` đơn | Chỉ inner condition | `comp.shrko` |
| Shiroprehension `;` đơn  | Chỉ outer condition (kiểu {x ∈ S ; Q}) | `comp.shrko` |
| Shiroprehension kết hợp | `\|` trước `;` — bắt buộc thứ tự | `comp.shrko` |
| Range literal | `[]int{1..5}` → `{1,2,3,4,5}` — closed range | `comp.shrko` |
| Đệ quy sâu | `fib(20)`, `factorial(10)` | `func.shrko` |
| Ép kiểu mất mát | `int(3.14)` → `3` | `cast.shrko` |
| Vòng lặp rỗng | `for range 0 {}` | `loop.shrko` |

### 6.3. shirocc là gì?

- **shirocc (Shiroko Compact Compiler)** là công cụ kiểm thử nội bộ do nhóm phát triển, lấy cảm hứng từ GCC (GNU Compiler Collection).
- Tên gọi "**Compact**" thể hiện triết lý thiết kế cốt lõi của ShirokoLang:
  - **Nhỏ gọn** (compact) — ít keyword, cú pháp tối giản
  - **Nhanh nhạy** (agile) — biên dịch nhanh, phản hồi tức thì
  - **Linh hoạt** (flexible) — dịch được sang nhiều ngôn ngữ đích (C, C++, Python, Go, hợp ngữ)

→ Ba đặc tính này khiến ShirokoLang phù hợp với tinh thần "**nén**" — gọn mà mạnh, ít mà đủ.

### 6.4. Self-hosting bước đầu

- Nhóm đã viết thành công **lexer của ShirokoLang bằng chính ShirokoLang** (`lexer.shrko`). Đây là bước đầu tiên hướng tới mục tiêu self-hosting hoàn chỉnh ở mục 9.
- So sánh output lexer Go vs lexer ShirokoLang trên 11 file test:
  khớp 100% token (số lượng, loại, giá trị).

## 7. Hạn chế và hướng phát triển
### 7.1 Hạn chế hiện tại
- Chưa có package manager.
- Chưa có generics (dự kiến v1.27).

### Ghi chú thiết kế (không phải hạn chế)
- Không có VM → không có GC riêng. Bộ nhớ do ngôn ngữ đích quản lý:
  Go/Python có GC sẵn; C/ASM dùng mô hình thủ công.
- Generic đã được hỗ trợ ở tầng kiểu (chi tiết ở mục 4).

### 7.2 Kế hoạch ngắn hạn (v1.27 - v1.30)
- Implement generics (đã có spec test `generics.shrko`).
- Thêm package manager tối giản.
- Viết parser self-hosting bằng ShirokoLang (bước 2 của self-hosting).

### 8. So sánh với ngôn ngữ khác
|Tiêu chí|ShirokoLang|Python|JavaScript|
|---|---|---|---|
|Cú pháp|đơn giản, dễ học|đơn giản, dễ, dài, ký tự thừa|khó hiểu, ký tự thừa|
|Range| Closed `[a,b]` | Half-open `[a,b)` | Không có |
|List comp|**Shiroprehension** (mô phỏng ký hiệu toán)|List comprehension|Array methods|
|Hiệu năng|Nhanh, biên dịch|Chậm, thông dịch|Chậm, nhưng có V8-engine|
|Hệ sinh thái|Chưa có|Khổng lồ (PyPi)|Khổng lồ (npm)|
|Dễ học|Rất dễ, ít keyword|Dễ, nhiều keyword|Trung bình|
|Kiểu dữ liệu|Tĩnh, an toàn|Động, dễ lỗi|Động|
|Mục đích chính|Đa dụng, giáo dục và nghiên cứu|Đa dụng|Web, full-stack|

### 9. Định hướng dài hạn

Trong tương lai, nhóm sẽ tiếp tục phát triển ShirokoLang theo đúng triết lý
**"Compact"** — giữ cho ngôn ngữ nhỏ gọn, nhanh nhạy và linh hoạt — đồng thời
hướng tới:

- **Self-hosting:** Dùng ShirokoLang viết trình biên dịch cho chính nó.
- **Hệ thống module hiện đại:** Quản lý module gọn gàng, dễ mở rộng.
- **Debugger hoàn chỉnh:** Hỗ trợ gỡ lỗi trực quan.
- **Tài liệu đầy đủ:** Giúp người mới học tiếp cận nhanh chóng.

Mốc version định hướng dài hạn:

- v26.11 (2026): package manager
- v26.12 (2026): parser self-hosted hoàn chỉnh
- v27.01 (2027): generics

Mục tiêu cuối cùng: trở thành một ngôn ngữ lập trình Việt Nam thực thụ,
thân thiện với người học và nhà nghiên cứu.

## 10. Phân chia nhiệm vụ

### 10.1 Trung Nghĩa — Technical Leader (10A6)

- Thiết kế ngôn ngữ: cú pháp, hệ thống kiểu, quy tắc ngữ nghĩa
- Viết compiler bằng Go (lexer, parser, semantic, lower, backend)
- Viết shirocc (Shiroko Compact Compiler)
- Viết lexer self-hosting bằng ShirokoLang (`lexer.shrko`)
- Soạn nội dung kỹ thuật của báo cáo

### 10.2 Trần Đào Gia Minh — Web Designer (12A1)

- Thiết kế website dự án
- Trình bày tài liệu và nhận diện thương hiệu

### 10.3 Công việc chung

- Kiểm thử và sửa lỗi
- Thảo luận thiết kế ngôn ngữ
