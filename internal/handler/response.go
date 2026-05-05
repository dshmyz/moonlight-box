package handler

import (
	"github.com/moonlight-box/registry/internal/response"
)

// 以下类型为 response 包中的类型，在此重新导出以保持向后兼容
type Response = response.Response
type PaginatedData = response.PaginatedData
type Pagination = response.Pagination

// 以下函数为 response 包中的函数，在此重新导出以保持向后兼容
var Success = response.Success
var SuccessWithPagination = response.SuccessWithPagination
var Created = response.Created
var NoContent = response.NoContent
var BadRequest = response.BadRequest
var Unauthorized = response.Unauthorized
var Forbidden = response.Forbidden
var NotFound = response.NotFound
var InternalError = response.InternalError
var Conflict = response.Conflict
var ErrorResponse = response.ErrorResponse
