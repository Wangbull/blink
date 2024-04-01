package blink

type Env struct {
	isSYS64 bool
	isDebug bool
}

var AppID = RandString(8)

func (env *Env) IsSYS64() bool {
	return env.isSYS64
}

func (env *Env) IsDebug() bool {
	return env.isDebug
}

func (env *Env) IsRelease() bool {
	return !env.isDebug
}
