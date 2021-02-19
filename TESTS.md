The tests suite here starts with a standard of gomock and ginkgo

for a basic understanding of these refer to:
- [ginkgo's website](https://onsi.github.io/ginkgo/)
- [gomocks github page](https://github.com/golang/mock)

The mocks we mostly use are generated using `make go-generate` which relies on `mockgen`.
This means you won't need to write your own mocks,
but know how to use the existing ones.

The next thing is to understand the structure of the mocks.
Each test, which is wrapped in an `It` block.
has usually one or more of these sections:
- `BeforeEach` block: to set default values
- `JustBeforeEach` block: to use the default values (which were set in the BeforeEach)

In addition to the specific tests, we set in the top of the test file two global `BeforeEach` and `JustBeforeEach` blocks.
These are used to set global variables and reduce code duplication.

Another feature that was added a while ago is the `./pkg/util/test/helper/helper.go:MockHelper` struct.
To understand more on this, [read the docs inside the package](./pkg/util/test/helper/helper.go)
