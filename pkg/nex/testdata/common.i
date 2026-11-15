	DEVICE ZXSPECTRUMNEXT
	; fill a 16K bank with a pattern: bank number, then a counter
	MACRO FILLBANK bank
	MMU 6 7, bank*2
	ORG $C000
	DUP 4096
	DB bank, low($), high($), $5A
	EDUP
	ENDM
