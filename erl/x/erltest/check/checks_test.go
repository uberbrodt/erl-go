package check

//NOTE: I see why the gotest.tools authors use interfaces for *testing.T in their
// assertions
// import (
// 	"testing"
//
// 	"gotest.tools/v3/assert"
// )
//
// func TestChain_ReturnsTrueIfAllSucceed(t *testing.T) {
// 	res := Chain(t, DeepEqual(t, 1, 1), Equal(t, "one", "one"))
//
// 	assert.Assert(t, res)
// }
//
// func TestEqual_DoesNotFail(t *testing.T) {
// 	res := Chain(t,
// 		DeepEqual(t, 1, 1),
// 		Equal(t, "one", "one"))
//
// 	assert.Assert(t, !res)
// }
//
// func TestChain_ReturnsFalseIfOneFails(t *testing.T) {
// 	res := Chain(t,
// 		DeepEqual(t, 1, 1),
// 		Equal(t, "foo", "bar"),
// 		Equal(t, "one", "one"))
//
// 	assert.Assert(t, res == false)
// }
