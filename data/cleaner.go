package data

type IClean interface {
	DoClean()
}

type IRefCount interface {
	AddRef()
	Release() bool
}
