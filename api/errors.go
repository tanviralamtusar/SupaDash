package api

import "github.com/gin-gonic/gin"

// errJSON writes a JSON error response with both "error" and "message" keys.
// Supabase Studio's toasts read err.message (see studio/patches/07-hcaptcha.patch),
// while some of our own handlers/tests read "error" — sending both keeps
// existing consumers working while making failures visible in the UI instead
// of showing "undefined".
func errJSON(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg, "message": msg})
}
