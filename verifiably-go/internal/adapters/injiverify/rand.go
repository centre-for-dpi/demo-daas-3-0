package injiverify

import "crypto/rand"

// cryptoReadFunc is the seam the tests override. Default is crypto/rand.
var cryptoReadFunc = rand.Read
