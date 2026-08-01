package handler

import "github.com/rs/zerolog"

var testJWTSecret = []byte("test-secret-for-handler-tests")

func testLoggerNop() zerolog.Logger {
	return zerolog.Nop()
}
