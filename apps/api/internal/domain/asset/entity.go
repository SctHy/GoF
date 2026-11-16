package domainasset

import "errors"

// ErrNotOwned 不区分不存在、类型不符和其他用户的资产，避免泄露归属。
var ErrNotOwned = errors.New("asset must be uploaded by the current user with the correct kind")
