package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
}

func ValidateRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) > 0 {
			return
		}

		if err := c.ShouldBindBodyWithJSON(nil); err != nil {
			return
		}
	}
}

func Validate[T any]() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req T
		if err := c.ShouldBindJSON(&req); err != nil {
			if validationErrs, ok := err.(validator.ValidationErrors); ok {
				fields := make(map[string]string)
				for _, e := range validationErrs {
					fields[e.Field()] = formatValidationError(e)
				}

				c.JSON(http.StatusBadRequest, gin.H{
					"code":    400,
					"message": "validation failed",
					"fields":  fields,
				})
				c.Abort()
				return
			}

			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "invalid request body",
			})
			c.Abort()
			return
		}

		c.Set("validated_request", req)
		c.Next()
	}
}

func formatValidationError(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return e.Field() + " is required"
	case "min":
		return e.Field() + " must be at least " + e.Param() + " characters"
	case "max":
		return e.Field() + " must be at most " + e.Param() + " characters"
	case "email":
		return e.Field() + " must be a valid email address"
	case "url":
		return e.Field() + " must be a valid URL"
	case "numeric":
		return e.Field() + " must be a number"
	case "oneof":
		return e.Field() + " must be one of: " + e.Param()
	default:
		return e.Field() + " validation failed on " + e.Tag()
	}
}

func GetValidatedRequest[T any](c *gin.Context) (T, bool) {
	req, exists := c.Get("validated_request")
	if !exists {
		var zero T
		return zero, false
	}
	typedReq, ok := req.(T)
	return typedReq, ok
}
