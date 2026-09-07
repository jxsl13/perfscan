package ps6102deep

import (
	"testing"
	"time"
)

// TestDeepKnownBranches pins linear traversal through non-threshold if
// statements. There is deliberately no wall-clock assertion: the depth makes
// recursively launching failure probes at every level requires
// combinatorially many redundant visits, while the intended traversal visits
// each branch once.
func TestDeepKnownBranches(t *testing.T) {
	if true {
		if true {
			if true {
				if true {
					if true {
						if true {
							if true {
								if true {
									if true {
										if true {
											if true {
												if true {
													if true {
														if true {
															if true {
																if true {
																	if true {
																		if true {
																			if true {
																				if true {
																					if true {
																						if true {
																							if true {
																								if true {
																									if true {
																										if true {
																											if true {
																												if true {
																													if true {
																														if true {
																															if true {
																																if true {
																																	if true {
																																		if true {
																																			if true {
																																				if true {
																																					if true {
																																						if true {
																																							if true {
																																								if true {
																																									if elapsed := time.Since(time.Now()); elapsed > time.Second { // want `performance threshold "elapsed > time.Second" in TestDeepKnownBranches remains reachable when testing.Short\(\) is true`
																																										t.Fatal("slow")
																																									}
																																								}
																																							}
																																						}
																																					}
																																				}
																																			}
																																		}
																																	}
																																}
																															}
																														}
																													}
																												}
																											}
																										}
																									}
																								}
																							}
																						}
																					}
																				}
																			}
																		}
																	}
																}
															}
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
}
