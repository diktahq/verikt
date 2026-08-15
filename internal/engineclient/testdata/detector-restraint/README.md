# detector-restraint

Code written the way a careful engineer writes it. **Every detector must stay
silent on this fixture.** Any finding here is a false positive, and the test
fails.

The counterpart to `detector-coverage`, which asserts all 14 detectors fire.
Detection was guarded and restraint was not, so a detector could become louder
forever and no test objected — which is how `global_mutable_state` came to be
wrong 21 times out of 22 on verikt's own source.

Add a case here whenever a detector reports something a competent reviewer would
not act on. A finding you cannot act on is not advice.
