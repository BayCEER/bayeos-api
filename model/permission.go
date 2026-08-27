package model

const (
	READ_PERMISSION  uint8 = 4
	WRITE_PERMISSION uint8 = 2
	GRANT_PERMISSION uint8 = 1
	FULL_PERMISSION  uint8 = READ_PERMISSION + WRITE_PERMISSION + GRANT_PERMISSION
)

func Readable(perm uint8) bool {
	return HasPermission(perm, READ_PERMISSION)
}

func Writeable(perm uint8) bool {
	return HasPermission(perm, WRITE_PERMISSION)
}

func Grantable(perm uint8) bool {
	return HasPermission(perm, GRANT_PERMISSION)
}

func HasPermission(c uint8, p uint8) bool {
	return (c & p) == p
}
