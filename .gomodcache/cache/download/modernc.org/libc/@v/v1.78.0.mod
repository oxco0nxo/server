module modernc.org/libc

go 1.26.0

retract v1.43.0

retract v1.67.5

retract v1.74.2 // freeaddrinfo leaks a locked ___lock entry; deadlocks name resolution, fixed in v1.74.4

retract v1.74.3 // freeaddrinfo leaks a locked ___lock entry; deadlocks name resolution, fixed in v1.74.4

retract v1.77.0 // Xnanf calls itself on linux; nan() and strtod or scanf of "nan" crash with a stack overflow

require (
	github.com/dustin/go-humanize v1.1.0
	github.com/google/uuid v1.6.0
	github.com/mattn/go-isatty v0.0.24
	github.com/ncruces/go-strftime v1.0.0
	golang.org/x/sys v0.48.0
	golang.org/x/tools v0.51.0
	modernc.org/cc/v4 v4.30.1
	modernc.org/ccgo/v4 v4.36.2
	modernc.org/fileutil v1.4.0
	modernc.org/goabi0 v0.2.0
	modernc.org/mathutil v1.7.1
	modernc.org/memory v1.12.1
)

require (
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	modernc.org/gc/v2 v2.6.5 // indirect
	modernc.org/gc/v3 v3.2.0 // indirect
	modernc.org/opt v0.2.0 // indirect
	modernc.org/sortutil v1.2.1 // indirect
	modernc.org/strutil v1.2.1 // indirect
	modernc.org/token v1.1.0 // indirect
)
