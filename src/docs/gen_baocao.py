#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Báo cáo kỹ thuật ShirokoLang — theo cấu trúc docs/baocaokythuattrungnghia.html"""

from docx import Document
from docx.shared import Pt, Cm, RGBColor
from docx.enum.text import WD_ALIGN_PARAGRAPH
from docx.enum.table import WD_TABLE_ALIGNMENT
from docx.oxml.ns import qn
from docx.oxml import OxmlElement


# ---------- helpers ----------

def H(doc, text, level):
    h = doc.add_heading(text, level=level)
    for r in h.runs:
        r.font.name = 'Times New Roman'
        r.font.color.rgb = RGBColor(0x1a, 0x1a, 0x1a)
    return h


def P(doc, text, bold=False, italic=False, center=False):
    p = doc.add_paragraph()
    if center:
        p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p.paragraph_format.space_after = Pt(6)
    r = p.add_run(text)
    r.font.name = 'Times New Roman'
    r.font.size = Pt(13)
    r.bold = bold
    r.italic = italic
    return p


def B(doc, text):
    p = doc.add_paragraph(style='List Bullet')
    r = p.add_run(text)
    r.font.name = 'Times New Roman'
    r.font.size = Pt(13)
    return p


def N(doc, text):
    p = doc.add_paragraph(style='List Number')
    r = p.add_run(text)
    r.font.name = 'Times New Roman'
    r.font.size = Pt(13)
    return p


def CODE(doc, text):
    p = doc.add_paragraph()
    p.paragraph_format.left_indent = Cm(0.5)
    p.paragraph_format.space_before = Pt(6)
    p.paragraph_format.space_after = Pt(6)
    r = p.add_run(text)
    r.font.name = 'Consolas'
    r.font.size = Pt(10)
    shd = OxmlElement('w:shd')
    shd.set(qn('w:val'), 'clear')
    shd.set(qn('w:color'), 'auto')
    shd.set(qn('w:fill'), 'F5F5F5')
    r._element.get_or_add_rPr().append(shd)
    return p


def TBL(doc, headers, rows):
    t = doc.add_table(rows=1, cols=len(headers))
    t.style = 'Light Grid Accent 1'
    t.alignment = WD_TABLE_ALIGNMENT.CENTER
    # Không cho phép cắt giữa dòng trong bảng
    for row in t.rows:
        trPr = row._tr.get_or_add_trPr()
        cant = OxmlElement('w:cantSplit')
        trPr.append(cant)
    for i, h in enumerate(headers):
        c = t.rows[0].cells[i]
        c.text = ''
        r = c.paragraphs[0].add_run(h)
        r.bold = True
        r.font.name = 'Times New Roman'
        r.font.size = Pt(12)
    for row in rows:
        cells = t.add_row().cells
        trPr = cells[0]._tc.getparent().get_or_add_trPr()
        cant = OxmlElement('w:cantSplit')
        trPr.append(cant)
        for i, v in enumerate(row):
            cells[i].text = ''
            r = cells[i].paragraphs[0].add_run(str(v))
            r.font.name = 'Times New Roman'
            r.font.size = Pt(12)
    return t


def cover(doc, text, size=13, bold=False, italic=False, sb=0, sa=0):
    p = doc.add_paragraph()
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p.paragraph_format.space_before = Pt(sb)
    p.paragraph_format.space_after = Pt(sa)
    p.paragraph_format.line_spacing = 1.15
    r = p.add_run(text)
    r.font.name = 'Times New Roman'
    r.font.size = Pt(size)
    r.bold = bold
    r.italic = italic
    return p


# ---------- setup ----------

doc = Document()
s = doc.sections[0]
s.page_height = Cm(29.7)
s.page_width = Cm(21.0)
s.left_margin = Cm(3.0)
s.right_margin = Cm(2.0)
s.top_margin = Cm(2.0)
s.bottom_margin = Cm(2.0)

st = doc.styles['Normal']
st.font.name = 'Times New Roman'
st.font.size = Pt(13)
st.element.rPr.rFonts.set(qn('w:eastAsia'), 'Times New Roman')


# ==================== TRANG BÌA ====================

cover(doc, 'SỞ GIÁO DỤC VÀ ĐÀO TẠO AN GIANG', 13, True, sa=2)
cover(doc, 'TRƯỜNG THPT NGUYỄN THẦN HIẾN', 13, True, sa=4)
cover(doc, '───────  ❖  ───────', 12)

cover(doc, 'CUỘC THI KHOA HỌC KỸ THUẬT', 13, True, sb=40, sa=2)
cover(doc, 'DÀNH CHO HỌC SINH TRUNG HỌC', 13, True, sa=2)
cover(doc, 'NĂM HỌC 2026 – 2027', 13, True)

cover(doc, '───────────', 12, sb=20, sa=20)
cover(doc, 'BÁO CÁO KỸ THUẬT', 16, True, sa=2)
cover(doc, 'NGÔN NGỮ LẬP TRÌNH SHIROKOLANG', 16, True)

cover(doc, 'Lĩnh vực: Phần mềm hệ thống', 12, sb=20)
cover(doc, 'Phiên bản: 26.11 (năm 2026)', 12, sb=6)

cover(doc, 'Tác giả:', 13, True, sb=15, sa=6)

info = doc.add_table(rows=2, cols=2)
info.alignment = WD_TABLE_ALIGNMENT.CENTER
info.autofit = False
for i, (name, role) in enumerate([
    ('Trần Trung Nghĩa', 'Technical Leader — Lớp 10A6'),
    ('Trần Đào Gia Minh', 'Web Designer — Lớp 12A1'),
]):
    c0, c1 = info.cell(i, 0), info.cell(i, 1)
    c0.width, c1.width = Cm(5.5), Cm(8.0)
    p0 = c0.paragraphs[0]
    p0.alignment = WD_ALIGN_PARAGRAPH.RIGHT
    r0 = p0.add_run('• ' + name)
    r0.font.name = 'Times New Roman'
    r0.font.size = Pt(13)
    r0.bold = True
    p1 = c1.paragraphs[0]
    r1 = p1.add_run(' — ' + role)
    r1.font.name = 'Times New Roman'
    r1.font.size = Pt(12)
    r1.italic = True

# ẩn viền bảng
borders = OxmlElement('w:tblBorders')
for edge in ('top', 'left', 'bottom', 'right', 'insideH', 'insideV'):
    el = OxmlElement('w:' + edge)
    el.set(qn('w:val'), 'none')
    el.set(qn('w:sz'), '0')
    borders.append(el)
info._tbl.tblPr.append(borders)

cover(doc, 'Đơn vị: Trường THPT Nguyễn Thần Hiến', 13, sb=20)
doc.add_paragraph()
cover(doc, 'An Giang, tháng 12 năm 2026', 13, italic=True)

doc.add_page_break()


# ==================== MỤC LỤC ====================

H(doc, 'MỤC LỤC', 1)
for line in [
    '1. Tổng quan (Overview)',
    '   1.1. Giới thiệu',
    '   1.2. Động lực',
    '   1.3. Đối tượng hướng đến',
    '',
    '2. Đặc điểm ngôn ngữ (Features)',
    '   2.1. Cú pháp cơ bản',
    '   2.2. Bảng toán tử và ký hiệu',
    '   2.3. EBNF',
    '   2.4. Thuật ngữ (Glossary)',
    '   2.5. Quy tắc ngữ nghĩa',
    '',
    '3. Kiến trúc hệ thống (Architecture)',
    '   3.1. Sơ đồ tổng quát',
    '   3.2. Các thành phần',
    '   3.3. Công nghệ sử dụng',
    '   3.4. Sử dụng',
    '',
    '4. Hệ thống kiểu',
    '   4.1. Generics (dự kiến v27.01)',
    '',
    '5. Ví dụ minh họa (Examples)',
    '6. Kiểm thử (Testing)',
    '7. Hạn chế và hướng phát triển',
    '8. So sánh với ngôn ngữ khác',
    '9. Định hướng dài hạn',
    '10. Phân chia nhiệm vụ',
    'Tài liệu tham khảo',
]:
    P(doc, line)

doc.add_page_break()


# ==================== 1. TỔNG QUAN ====================

H(doc, '1. Tổng quan (Overview)', 1)

H(doc, '1.1. Giới thiệu', 2)
P(doc, 'ShirokoLang là một ngôn ngữ lập trình biên dịch được thiết kế với mục tiêu dễ học, dễ sử dụng cho lứa tuổi học sinh và nhà nghiên cứu kỹ thuật.')
P(doc, 'Triết lý thiết kế cốt lõi có thể tóm gọn trong một từ: "Compact" (nhỏ gọn) — nhỏ gọn, nhanh nhạy, linh hoạt. Đây cũng là tinh thần xuyên suốt từ cú pháp, hệ thống kiểu, đến công cụ kiểm thử shirocc (Shiroko Compact Compiler).')
P(doc, 'Ngôn ngữ được phát triển như một dự án học tập về nguyên lý ngôn ngữ lập trình và trình biên dịch (compiler).')

H(doc, '1.2. Động lực', 2)
B(doc, 'Việt Nam hiện nay chưa có ngôn ngữ lập trình thân thiện với người học và mục tiêu nghiên cứu. Mặc dù Python hay C++ đã đáp ứng phần nào nhu cầu hiện có, nhưng cú pháp của chúng khá phức tạp và không hợp với lối đọc hiểu ngắn gọn, hàm súc của người học. Python dùng range half-open [a,b) gây nhầm lẫn cho người mới; ShirokoLang chọn closed range [a,b] nhất quán.')
B(doc, 'Ngôn ngữ lập trình thân thiện là một vấn đề chưa được giải quyết tốt đối với ngành giáo dục và các ngành nghiên cứu hiện tại.')

H(doc, '1.3. Đối tượng hướng đến', 2)
B(doc, 'Học sinh')
B(doc, 'Người mới học lập trình')
B(doc, 'Lập trình viên muốn thử ngôn ngữ mới')
B(doc, 'Nhà nghiên cứu cần một ngôn ngữ đơn giản')


# ==================== 2. ĐẶC ĐIỂM NGÔN NGỮ ====================

H(doc, '2. Đặc điểm ngôn ngữ (Features)', 1)
B(doc, 'Kiểu tĩnh')
B(doc, 'Lập trình hướng đối tượng')
B(doc, 'Lập trình hàm')

H(doc, '2.1. Cú pháp cơ bản', 2)

P(doc, 'Khai báo biến:', bold=True)
CODE(doc, 'let x = 10\nconst PI = 3.14')

P(doc, 'Hàm:', bold=True)
CODE(doc, 'func cong(a, b int) int {\n  return a + b\n}')

P(doc, 'Giao diện và cấu trúc:', bold=True)
CODE(doc, 'interface GiaoDien {\n  area() float\n}\n\nstruct HinhVuong {\n  dai int\n}\n\nfunc HinhVuong.area() float {\n  return self.dai * self.dai\n}')

P(doc, 'Vòng lặp và điều kiện:', bold=True)
CODE(doc, 'if x > 0 {}\n\nfor i = 0, i < 10, i++ {}\nfor range 10 {}\n\nlet ds = []int{1, 2, 3}\n\nfor d = iter ds {}\nfor i, d = iter ds {}\n\nlet x = 7\n\nmatch x {\n  10 => "ten"\n  5..9 => "range 5..9"\n  _ => "little"\n}')

P(doc, 'Danh sách:', bold=True)
CODE(doc, 'let danhSach = []int{1, 2, 3}')

P(doc, 'Shiroprehension (list comprehension):', bold=True)
CODE(doc, '// lọc\nlet comp = []int{x @ danhSach | x % 2 == 0; x < 30}\n\n// lấy phần theo điều kiện biên\nlet cmp = []int{x @ danhSach; x > 2}')

P(doc, 'Dùng module:', bold=True)
CODE(doc, 'import {\n  "std/fmt"\n}')

P(doc, 'Ép kiểu:', bold=True)
CODE(doc, 'const PI = 3.14\nlet x = int(PI)')

P(doc, 'Ví dụ Hello World:', bold=True)
CODE(doc, 'package main\n\nimport {\n  "std/fmt"\n}\nfunc main() {\n  fmt.println("Hello, ShirokoLang!")\n}')

H(doc, '2.2. Bảng toán tử và ký hiệu', 2)
TBL(doc,
    ['Ký hiệu', 'Ý nghĩa', 'Ví dụ'],
    [
        ['=', 'gán', 'let x = 10'],
        ['|', 'filter trong (inner) — kiểm tra sau ;', '[]int{x @ ds | x % 2 == 0}'],
        [';', 'filter ngoài (outer) — kiểm tra trước |', '[]int{x @ ds; x < 30}'],
        [',', 'phân tách tham số / phần tử', 'fmt.println(1, 2, 3)'],
        ['-', 'âm', '-a'],
        ['!', 'phủ định', '!điều_kiện'],
        ['true, false', 'literal bool', 'let x = false'],
        ['||, &&', 'hoặc, và', 'if a > 0 || b > 0 {}'],
        ['+ - * / %', 'số học', 'a + b'],
        ['== != < > <= >=', 'so sánh', 'x == 10'],
        ['..', 'khoảng', '1..10, 5..9'],
        ['=>', 'match arm', '10 => "ten"'],
        ['@', '"thuộc" — nguồn trong Shiroprehension (∈)', 'x @ danhSach'],
        ['++', 'tăng', 'i++'],
    ])

H(doc, '2.3. EBNF', 2)
P(doc, 'Quy ước ký hiệu EBNF dùng trong tài liệu này:')
TBL(doc,
    ['Ký hiệu', 'Ý nghĩa'],
    [
        ['=', 'định nghĩa rule'],
        ['|', 'hoặc'],
        [',', 'nối tiếp'],
        ['{ ... }', 'lặp 0 hoặc nhiều lần'],
        ['[ ... ]', 'tùy chọn (0 hoặc 1)'],
        ['( ... )', 'nhóm'],
        ['"..."', 'terminal (literal)'],
        ['(* ... *)', 'comment'],
    ])

P(doc, 'b. Syntax grammar (cú pháp):', bold=True)
CODE(doc, '''(* ===== Chương trình ===== *)
program       = packageClause , { importClause } , { topDecl } ;
packageClause = "package" , identifier ;
importClause  = "import" , "{" , { string } , "}" ;

topDecl       = varDecl | constDecl | function | method
              | structDecl | interfaceDecl ;

(* ===== Khai báo biến / hằng ===== *)
varDecl       = "let" , identifier , [ type ] , "=" , expr ;
constDecl     = "const" , identifier , [ type ] , "=" , expr ;

(* ===== Hàm ===== *)
function      = "func" , identifier , "(" , [ params ] , ")" ,
                [ type ] , block ;
method        = "func" , receiverType , "." , methodName ,
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

lambdaExpr    = "func" , "(" , [ params ] , ")" , [ type ] , block ;

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
   - `|` (sao cho) là điều kiện trong (inner).
   - `;` (và)     là điều kiện ngoài (outer).
   - Cú pháp viết: `|` trước, `;` sau.
   - Thứ tự thực thi: `;` chạy trước `|` (kiểm tra điều kiện ngoài
     trước để loại sớm, giảm chi phí). *)

(* ===== Kiểu ===== *)
type          = primitiveType | listType | mapType ;
primitiveType = "int" | "float" | "string" | "bool" | "byte" | "any" ;
listType      = "[]" , type ;
mapType       = "map" , "[" , type , "]" , type ;

(* ===== Tham số cho lời gọi hàm ===== *)
args          = expr , { "," , expr } ;''')

H(doc, '2.4. Thuật ngữ (Glossary)', 2)
TBL(doc,
    ['Thuật ngữ', 'Ký hiệu / Cú pháp', 'Nghĩa'],
    [
        ['Shiroprehension', '[]T{...}', 'Cú pháp tạo mảng, mô phỏng ký hiệu tập hợp'],
        ['thuộc', '@', 'Nguồn trong Shiroprehension (∈)'],
        ['sao cho', '|', 'Mở đầu điều kiện lọc'],
        ['và', ';', 'Nối điều kiện lọc'],
        ['shirocc', '—', 'Shiroko Compact Compiler'],
        ['self', '—', 'Tham chiếu receiver trong method'],
        ['range đóng', 'a..b', '[a,b], bao gồm cả 2 đầu'],
    ])

H(doc, '2.5. Quy tắc ngữ nghĩa', 2)
TBL(doc,
    ['Quy tắc', 'Mô tả'],
    [
        ['Range', 'a..b là closed [a,b], áp dụng nhất quán'],
        ['Match overlap', 'Arm giao nhau → lỗi compile'],
        ['Shiroprehension', '; (và) kiểm tra trước | (sao cho)'],
        ['Ép kiểu', 'widening tự động, narrowing tường minh'],
    ])

doc.add_page_break()


# ==================== 3. KIẾN TRÚC ====================

H(doc, '3. Kiến trúc hệ thống (Architecture)', 1)

H(doc, '3.1. Sơ đồ tổng quát', 2)
CODE(doc, '''+------------------- shiroko/core -------------------+
|                                                    |
|  .shrko -> Lexer -> Parser -> AST -> Semantic      |
|         -> Lower -> IR                             |
|                                                    |
+------------------------+---------------------------+
                         | IR
                         v
+------------------- shiroko/backend ----------------+
|                                                    |
|   +--------+  +--------+  +--------+  +--------+   |
|   |  Go    |  | Python |  |   C    |  |  ASM   |   |
|   |(chinh) |  | (thu)  |  | (thu)  |  | (thu)  |   |
|   +--------+  +--------+  +--------+  +--------+   |
|                                                    |
+----------------------------------------------------+

Hai hướng thực thi chính:
  - Compile-to-Go:        Emitter sinh mã Go
  - Interpreter trên VM:  register-based, peephole optimizer''')

H(doc, '3.2. Các thành phần', 2)

P(doc, 'a. Lexer (bộ phân tích từ vựng)', bold=True)
B(doc, 'Chuyển chuỗi ký tự thành danh sách các token')
B(doc, 'Xử lý: số, chuỗi, từ khóa, toán tử, comment')
B(doc, 'Công cụ: viết tay')

P(doc, 'b. Parser (bộ phân tích cú pháp)', bold=True)
B(doc, 'Parser được viết tay hoàn toàn (hand-written), không sử dụng go/parser hay generator.')
B(doc, 'Kết hợp Recursive Descent cho statement/declaration và Pratt Parser cho expression để xử lý độ ưu tiên toán tử gọn gàng.')
B(doc, 'Xây dựng AST từ Token.')

P(doc, 'c. Semantic Checker (bộ phân tích ngữ nghĩa)', bold=True)
B(doc, 'Phân tích range coverage cho match arm: báo lỗi khi arm giao với arm trước (kể cả giao tại biên).')
B(doc, 'Kiểm tra kiểu tĩnh, ép kiểu tự động/tường minh.')
B(doc, 'Kiểm tra khai báo trùng lặp, chưa khai báo.')
B(doc, 'Kiểm tra nhập các module.')
B(doc, 'Gợi ý sửa lỗi bằng thuật toán Levenshtein.')

P(doc, 'd. Lower (bộ chuyển đổi AST)', bold=True)
B(doc, 'Lặp qua AST dịch sang IR.')

P(doc, 'e. Hai hướng thực thi chính', bold=True)
B(doc, 'Compile-to-Go: Emitter sinh mã Go từ IR, chạy với go run / go build.')
B(doc, 'Interpreter trên register-based VM: chạy trực tiếp IR, có peephole optimizer và constant folding.')

P(doc, 'Các backend khác (thử nghiệm):', bold=True)
TBL(doc,
    ['Backend', 'Trạng thái'],
    [
        ['Go (Compile-to-Go)', 'Hoàn chỉnh'],
        ['Interpreter (register-based VM)', 'Hoàn chỉnh'],
        ['Python', 'Thử nghiệm'],
        ['C', 'Thử nghiệm'],
        ['ASM', 'Thử nghiệm'],
    ])

H(doc, '3.3. Công nghệ sử dụng', 2)
B(doc, 'Ngôn ngữ viết compiler: Go')
B(doc, 'Thư viện thứ ba: không phụ thuộc bên thứ ba')
B(doc, 'Thư viện Go: fmt, strings, strconv, bytes')
B(doc, 'Build Tool: gc')

H(doc, '3.4. Sử dụng', 2)
CODE(doc, '$ shirocc compile go main.shrko main.go')
P(doc, 'Liên kết dự án:')
B(doc, 'GitHub: https://github.com/TebeeDeveloper/shirokolang')

doc.add_page_break()


# ==================== 4. HỆ THỐNG KIỂU ====================

H(doc, '4. Hệ thống kiểu', 1)
B(doc, 'Các kiểu dữ liệu nguyên thủy: int, float, string, bool, byte, any')
B(doc, 'Các kiểu dữ liệu mảng: []T')
B(doc, 'Các kiểu dữ liệu từ điển: map[K]V')
B(doc, 'Kiểm tra kiểu: tĩnh')
B(doc, 'Cách ép kiểu: tự động lẫn tường minh')

H(doc, '4.1. Generics (dự kiến v27.01)', 2)
B(doc, 'Hiện chưa hỗ trợ generic function/struct.')
B(doc, 'Kiến trúc đã sẵn sàng — chỉ cần thêm vài dòng ở parser + semantic.')
B(doc, 'Dự kiến: func f[T](x T) T, struct Box[T] { value T }.')


# ==================== 5. VÍ DỤ ====================

H(doc, '5. Ví dụ minh họa (Examples)', 1)

H(doc, '5.1. Tính giai thừa', 2)
CODE(doc, 'func factorial(n int) int {\n  if n <= 1 { return 1 }\n  return n * factorial(n - 1)\n}')

H(doc, '5.2. Fibonacci', 2)
CODE(doc, 'func fib(n int) int {\n  if n < 2 { return n }\n  return fib(n-1) + fib(n-2)\n}')

H(doc, '5.3. Ứng dụng thực tế', 2)
B(doc, 'Script tự động hóa')
B(doc, 'Tính toán')
B(doc, 'Chương trình ví dụ về lập trình đối tượng tách biệt')
B(doc, 'Viết mã phục vụ nghiên cứu')
B(doc, 'Sử dụng để thực hiện các cuộc thi lập trình hiện đại')


# ==================== 6. KIỂM THỬ ====================

H(doc, '6. Kiểm thử (Testing)', 1)

H(doc, '6.1. Danh sách test case', 2)
B(doc, 'Số lượng test case: 12 PASS + 1 SKIP')
B(doc, 'Công cụ test: shirocc (Shiroko Compact Compiler)')

TBL(doc,
    ['STT', 'File test', 'Nội dung kiểm tra', 'Kết quả'],
    [
        ['1', 'main.shrko', 'HelloWorld, import module std/fmt', 'PASS'],
        ['2', 'comp.shrko', 'Shiroprehension: lọc |, ;, range literal', 'PASS'],
        ['3', 'lexer.shrko', 'Token hóa: số, chuỗi, comment, toán tử; lexer self-hosting', 'PASS'],
        ['4', 'match.shrko', 'Match + range 1..10, chồng lấn 5..15, bao trùm 1..20 + 5..15', 'PASS'],
        ['5', 'var.shrko', 'Khai báo let, const, ép kiểu tường minh int(PI)', 'PASS'],
        ['6', 'arith.shrko', 'Toán tử số học, độ ưu tiên, kết hợp + - * / %', 'PASS'],
        ['7', 'logic.shrko', 'Toán tử logic &&, ||, !, so sánh == != < > <= >=', 'PASS'],
        ['8', 'func.shrko', 'Hàm, đệ quy (factorial, fib), tham số nhiều kiểu', 'PASS'],
        ['9', 'struct.shrko', 'struct, interface, method có receiver self', 'PASS'],
        ['10', 'loop.shrko', 'for range, for cổ điển, for iter (có/không index)', 'PASS'],
        ['11', 'cast.shrko', 'Ép kiểu int(float), float(int), string(int)', 'PASS'],
        ['12', 'lambda.shrko', 'Lambda expression, truyền hàm như tham số', 'PASS'],
        ['13', 'generics.shrko', 'Generics (dự kiến v27.01)', 'SKIP'],
    ])
P(doc, 'Lưu ý: các file test nằm ở mục tests/ của dự án.', italic=True)

H(doc, '6.2. Edge case đã xử lý', 2)
TBL(doc,
    ['Edge case', 'Mô tả', 'File test'],
    [
        ['Match chồng lấn', 'Nhiều arm cùng khớp một giá trị — báo lỗi chồng lấn', 'match.shrko'],
        ['Match bao trùm', 'Range 1..20 và 5..15 — báo lỗi chồng lấn', 'match.shrko'],
        ['Overlap tại biên', '0..5 và 5..6.5 giao tại 5.0 — báo lỗi chồng lấn', 'match.shrko'],
        ['Match mặc định', '_ bắt mọi giá trị còn lại', 'match.shrko'],
        ['Range closed', '1..10 bao gồm 10, khác Python', 'match.shrko'],
        ['Shiroprehension | đơn', 'Chỉ inner condition', 'comp.shrko'],
        ['Shiroprehension ; đơn', 'Chỉ outer condition (kiểu {x ∈ S ; Q})', 'comp.shrko'],
        ['Shiroprehension kết hợp', '| trước ; — bắt buộc thứ tự', 'comp.shrko'],
        ['Range literal', '[]int{1..5} → {1,2,3,4,5} — closed range', 'comp.shrko'],
        ['Đệ quy sâu', 'fib(20), factorial(10)', 'func.shrko'],
        ['Ép kiểu mất mát', 'int(3.14) → 3', 'cast.shrko'],
        ['Vòng lặp rỗng', 'for range 0 {}', 'loop.shrko'],
    ])

H(doc, '6.3. shirocc là gì?', 2)
B(doc, 'shirocc (Shiroko Compact Compiler) là công cụ kiểm thử nội bộ do nhóm phát triển, lấy cảm hứng từ GCC (GNU Compiler Collection).')
B(doc, 'Tên gọi "Compact" thể hiện triết lý thiết kế cốt lõi của ShirokoLang: nhỏ gọn (compact) — ít keyword, cú pháp tối giản; nhanh nhạy (agile) — biên dịch nhanh, phản hồi tức thì; linh hoạt (flexible) — dịch được sang nhiều ngôn ngữ đích.')
P(doc, '→ Ba đặc tính này khiến ShirokoLang phù hợp với tinh thần "nén" — gọn mà mạnh, ít mà đủ.')

H(doc, '6.4. Self-hosting bước đầu', 2)
B(doc, 'Nhóm đã viết thành công lexer của ShirokoLang bằng chính ShirokoLang (lexer.shrko). Đây là bước đầu tiên hướng tới mục tiêu self-hosting hoàn chỉnh ở mục 9.')
B(doc, 'So sánh output lexer Go vs lexer ShirokoLang trên 11 file test: khớp 100% token (số lượng, loại, giá trị).')


# ==================== 7. HƯỚNG DẪN SỬ DỤNG ====================

H(doc, '7. Hướng dẫn sử dụng', 1)

H(doc, '7.1 Build file thực thi bằng go', 2)

P(doc, 'Hướng dẫn cho linux')

CODE(doc, '''$ go build -o scc cmd/shirocc/main.go

$ GOOS=js GOARCH=wasm go build -o cmd/shiroweb/shiroko.wasm cmd/shiroweb/main.go
$ go build -o cmd/shiroweb/shiroweb cmd/shiroweb/serve.go
''')

P(doc, 'tương tự với windows, chỉ đổi từ không đuôi thành đuôi .exe khi biên dịch thuần')


# ==================== 7. HẠN CHẾ ====================

H(doc, '7. Hạn chế và hướng phát triển', 1)

H(doc, '7.1. Hạn chế hiện tại', 2)
B(doc, 'Chưa có package manager.')
B(doc, 'Chưa có generics (dự kiến v27.01).')

P(doc, 'Ghi chú thiết kế (không phải hạn chế):', bold=True)
B(doc, 'Không có VM riêng cho Go backend → không có GC riêng. Bộ nhớ do ngôn ngữ đích quản lý: Go/Python có GC sẵn; C/ASM dùng mô hình thủ công.')
B(doc, 'Generic đã được hỗ trợ ở tầng kiểu (chi tiết ở mục 4).')

H(doc, '7.2. Kế hoạch ngắn hạn (v26.12 – v27.01)', 2)
B(doc, 'Implement generics (đã có spec test generics.shrko).')
B(doc, 'Thêm package manager tối giản.')
B(doc, 'Viết parser self-hosting bằng ShirokoLang (bước 2 của self-hosting).')


# ==================== 8. SO SÁNH ====================

H(doc, '8. So sánh với ngôn ngữ khác', 1)
TBL(doc,
    ['Tiêu chí', 'ShirokoLang', 'Python', 'JavaScript'],
    [
        ['Cú pháp', 'đơn giản, dễ học', 'đơn giản, dễ, dài, ký tự thừa', 'khó hiểu, ký tự thừa'],
        ['Range', 'Closed [a,b]', 'Half-open [a,b)', 'Không có'],
        ['List comp', 'Shiroprehension (mô phỏng ký hiệu toán)', 'List comprehension', 'Array methods'],
        ['Hiệu năng', 'Nhanh, biên dịch', 'Chậm, thông dịch', 'Chậm, nhưng có V8-engine'],
        ['Hệ sinh thái', 'Chưa có', 'Khổng lồ (PyPi)', 'Khổng lồ (npm)'],
        ['Dễ học', 'Rất dễ, ít keyword', 'Dễ, nhiều keyword', 'Trung bình'],
        ['Kiểu dữ liệu', 'Tĩnh, an toàn', 'Động, dễ lỗi', 'Động'],
        ['Mục đích chính', 'Đa dụng, giáo dục và nghiên cứu', 'Đa dụng', 'Web, full-stack'],
    ])


# ==================== 9. ĐỊNH HƯỚNG ====================

H(doc, '9. Định hướng dài hạn', 1)
P(doc, 'Trong tương lai, nhóm sẽ tiếp tục phát triển ShirokoLang theo đúng triết lý "Compact" — giữ cho ngôn ngữ nhỏ gọn, nhanh nhạy và linh hoạt — đồng thời hướng tới:')
B(doc, 'Self-hosting: Dùng ShirokoLang viết trình biên dịch cho chính nó.')
B(doc, 'Hệ thống module hiện đại: Quản lý module gọn gàng, dễ mở rộng.')
B(doc, 'Debugger hoàn chỉnh: Hỗ trợ gỡ lỗi trực quan.')
B(doc, 'Tài liệu đầy đủ: Giúp người mới học tiếp cận nhanh chóng.')

P(doc, 'Mốc version định hướng dài hạn:')
B(doc, 'v26.11 (2026): package manager')
B(doc, 'v26.12 (2026): parser self-hosted hoàn chỉnh')
B(doc, 'v27.01 (2027): generics')

P(doc, 'Mục tiêu cuối cùng: trở thành một ngôn ngữ lập trình Việt Nam thực thụ, thân thiện với người học và nhà nghiên cứu.')


# ==================== 10. PHÂN CHIA NHIỆM VỤ ====================

H(doc, '10. Phân chia nhiệm vụ', 1)

H(doc, '10.1. Trung Nghĩa — Technical Leader (10A6)', 2)
B(doc, 'Thiết kế ngôn ngữ: cú pháp, hệ thống kiểu, quy tắc ngữ nghĩa')
B(doc, 'Viết compiler bằng Go (lexer, parser, semantic, lower, emitter)')
B(doc, 'Viết interpreter (register-based VM)')
B(doc, 'Viết shirocc (Shiroko Compact Compiler)')
B(doc, 'Viết lexer self-hosting bằng ShirokoLang (lexer.shrko)')
B(doc, 'Soạn nội dung kỹ thuật của báo cáo')

H(doc, '10.2. Trần Đào Gia Minh — Web Designer (12A1)', 2)
B(doc, 'Thiết kế website dự án (Playground WASM)')
B(doc, 'Trình bày tài liệu và nhận diện thương hiệu')

H(doc, '10.3. Công việc chung', 2)
B(doc, 'Kiểm thử và sửa lỗi')
B(doc, 'Thảo luận thiết kế ngôn ngữ')


# ==================== TÀI LIỆU THAM KHẢO ====================

H(doc, 'TÀI LIỆU THAM KHẢO', 1)
for t in [
    'Go Team. (2026). The Go Programming Language Specification. https://go.dev/ref/spec',
    'WebAssembly Community Group. (2026). WebAssembly Specification. https://webassembly.github.io/spec/',
    'Monaco Editor. (2026). Monaco Editor Documentation. https://microsoft.github.io/monaco-editor/',
    'GitHub Repository: https://github.com/TebeeDeveloper/shirokolang',
]:
    N(doc, t)


# ==================== LƯU ====================

out = 'BAO_CAO_KHKT_ShirokoLang.docx'
doc.save(out)
print(f'✅ Đã tạo: {out}')
print(f'   Mở: soffice --writer {out}')
