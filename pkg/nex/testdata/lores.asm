	INCLUDE "common.i"
	FILLBANK 5
	ORG $8000
start:	jr start
	SAVENEX OPEN "lores.nex", start
	SAVENEX SCREEN LR
	SAVENEX CLOSE
