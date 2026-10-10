big_u_value		\\U{hex_digit}{hex_digit}{hex_digit}{hex_digit}{hex_digit}{hex_digit}{hex_digit}{hex_digit}
binary_digit		[01]
binary_digits		{binary_digit}(_?{binary_digit})*
binary_lit		0[bB]_?{binary_digits}
byte_value		{octal_byte_value}|{hex_byte_value}
decimal_digit		[0-9]
decimal_digits		{decimal_digit}(_?{decimal_digit})*
decimal_exponent	[eE][-+]?{decimal_digits}
decimal_float_lit	{decimal_digits}"."{decimal_digits}?{decimal_exponent}?|{decimal_digits}{decimal_exponent}|"."{decimal_digits}{decimal_exponent}?
decimal_lit		0|[1-9](_?{decimal_digits})?
escaped_char		\\[abfnrtv\\'\"]
float_lit		{decimal_float_lit}|{hex_float_lit}
hex_byte_value		\\x{hex_digit}{hex_digit}
hex_digit		[0-9A-Fa-f]
hex_digits		{hex_digit}(_?{hex_digit})*
hex_float_lit		0[xX]{hex_mantissa}{hex_exponent}
hex_exponent		[pP][-+]?{decimal_digits}
hex_lit			0[xX]_?{hex_digits}
hex_mantissa		_?{hex_digits}"."{hex_digits}?|_?{hex_digits}|"."{hex_digits}
identifier		{letter}({letter}|{unicode_digit})*
imaginary_lit		({decimal_digits}|{int_lit}|{float_lit})i
int_lit			{decimal_lit}|{binary_lit}|{octal_lit}|{hex_lit}
interpreted_string_lit	\"({unicode_value}|{byte_value})*\"
letter			[a-zA-Z_]|{unicode_letter}
little_u_value		\\u{hex_digit}{hex_digit}{hex_digit}{hex_digit}
octal_byte_value	\\{octal_digit}{octal_digit}{octal_digit}
octal_digit		[0-7]
octal_digits		{octal_digit}(_?{octal_digit})*
octal_lit		0[oO]?_?{octal_digits}
raw_string_lit		`({unicode_char}|\n)*`
rune_lit		'({unicode_value}|{byte_value})'
string_lit		{raw_string_lit}|{interpreted_string_lit}
unicode_char		[^\n]
unicode_digit		\x80
unicode_letter		\x81
unicode_value		{unicode_char}|{little_u_value}|{big_u_value}|{escaped_char}
white			[ \n\r\t]+

%%

"!"
"!="
"%"
"%="
"&"
"&&"
"&="
"&^"
"&^="
"("
")"
"*"
"*="
"+"
"++"
"+="
","
"-"
"--"
"-="
"."
".."
"..."
"/"
"/="
":"
":="
";"
"<"
"<-"
"<<"
"<<="
"<="
"="
"=="
">"
">="
">>"
">>="
"["
"]"
"^"
"^="
"{"
"|"
"|="
"||"
"}"
"~"

break
case
chan
const
continue
default
defer
else
fallthrough
for
func
go
goto
if
import
interface
map
package
range
return
select
struct
switch
type
var

{float_lit}
{identifier}
{imaginary_lit}
{int_lit}
{rune_lit}
{string_lit}
{white}
