# Issue #891 explicit-bound fallback coverage

PS6135 covers the pre-existing GoAI `Decoder.binElem` optional-interface
wrapper. Its complete function and complete `binaryNRecorder` capability are
unchanged between parent `74a7c5c923b25aa35773bb9e907b76aba04a553a` and PR1210
merge `ec20269a20e2028ec10aa02cd9d26095e8aa161b`. The byte-pinned fixture retains
both unchanged declarations; its harness contains only types and the selected
`recorder.Binary` signature. This is a typed replay of the whole wrapper,
not a full Decoder/provider/build-path execution replay.

Source proves exact optional capability/provider identity, positive rows and
width guards, matching buffer/operation roles, the product passed to BinaryN,
and the same provider's fallback call with no active extent. The API contract
supplies capacity-wide work, active-prefix semantics, smaller active workloads,
unobserved tail, provider/build coverage, shape/errors and synchronization.
Runtime capacity, capability absence on a measured provider and an isolated
performance win are not source-derived facts.

PS6106's concrete-provider bounded producer/consumer grammar does not accept
this interface wrapper. PS6132 constructor-residency guidance and the separate
strided-bound guard scope do not replace this source/contract predicate.

The candidate is conditional on valid positive nonoverflowing active geometry, absent capability,
and retained capacity greater than the active extent. Invalid/nonpositive or
overflowing shape behavior must be preserved, not silently turned into an
optimization. No automatic fix is offered.

The [owner issue](https://github.com/jxsl13/perfscan/issues/891) and
[GoAI PR1210](https://github.com/jxsl13/goai/pull/1210) attribute about 1.366x
median StepNLast16 eager/lazy results with unchanged 83 allocs/op to the complete
exact-residency/backend campaign. This is not an isolated binElem fallback
measurement; PS6135 makes no measured replacement-gain claim.
