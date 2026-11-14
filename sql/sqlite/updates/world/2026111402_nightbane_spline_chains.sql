-- Nightbane spline chains (Karazhan, boss_nightbane.cpp: MoveAlongSplineChain
-- legs). Rows transcribed verbatim from TDB 335.26091
-- script_spline_chain_meta / script_spline_chain_waypoints for entry 17225
-- (Nightbane): chain 1 INTRO_START (3689ms, 2 wps), chain 2 INTRO_END
-- (26021ms, 10 wps), chain 3 INTRO_LANDING (2697ms, 2 wps), chain 4
-- SECOND_LANDING (15920ms, 2 wps: the descent to the landing point),
-- chain 5 PHASE_TWO (15920ms, 7 wps: FlyPosition -> landing approach).
-- The Go loader (loadSplineChains, engine/world/spline_chain.go) reads
-- exactly these tables; without the rows MoveAlongSplineChain returns
-- false and the Lua falls back to its 5s timers, so the phase-two legs
-- (EVENT_LAND -> chain 5 -> EVENT_END_PHASE_TWO 1ms -> chain 4 ->
-- EVENT_LANDED 3s) never fly. Chains 1-3 seed the intro machine too
-- (unported ACTION_SUMMON); chains 4-5 are the ones the Lua uses.
-- The NOT EXISTS guard keeps the migration a no-op on a deployment DB
-- already populated by a full TDB import (the tables carry a composite
-- primary key, so a plain INSERT would fail on re-run).
INSERT INTO script_spline_chain_meta (entry, chainId, splineId, expectedDuration, msUntilNext, velocity)
SELECT 17225, 1, 0, 3689, 0, 0
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_meta WHERE entry = 17225 AND chainId = 1 AND splineId = 0);
INSERT INTO script_spline_chain_meta (entry, chainId, splineId, expectedDuration, msUntilNext, velocity)
SELECT 17225, 2, 0, 26021, 0, 0
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_meta WHERE entry = 17225 AND chainId = 2 AND splineId = 0);
INSERT INTO script_spline_chain_meta (entry, chainId, splineId, expectedDuration, msUntilNext, velocity)
SELECT 17225, 3, 0, 2697, 0, 0
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_meta WHERE entry = 17225 AND chainId = 3 AND splineId = 0);
INSERT INTO script_spline_chain_meta (entry, chainId, splineId, expectedDuration, msUntilNext, velocity)
SELECT 17225, 4, 0, 15920, 0, 0
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_meta WHERE entry = 17225 AND chainId = 4 AND splineId = 0);
INSERT INTO script_spline_chain_meta (entry, chainId, splineId, expectedDuration, msUntilNext, velocity)
SELECT 17225, 5, 0, 15920, 0, 0
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_meta WHERE entry = 17225 AND chainId = 5 AND splineId = 0);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 1, 0, 0, -11003.7, -1760.19, 140.253
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 1 AND splineId = 0 AND wpId = 0);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 1, 0, 1, -11018.9, -1797.27, 172.852
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 1 AND splineId = 0 AND wpId = 1);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 2, 0, 0, -11018.9, -1797.27, 172.852
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 2 AND splineId = 0 AND wpId = 0);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 2, 0, 1, -11105.8, -1875.9, 158.978
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 2 AND splineId = 0 AND wpId = 1);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 2, 0, 2, -11175.1, -1857.22, 101.006
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 2 AND splineId = 0 AND wpId = 2);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 2, 0, 3, -11296.9, -1764.53, 101.006
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 2 AND splineId = 0 AND wpId = 3);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 2, 0, 4, -11258.9, -1722.37, 101.006
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 2 AND splineId = 0 AND wpId = 4);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 2, 0, 5, -11176.8, -1809.6, 101.006
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 2 AND splineId = 0 AND wpId = 5);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 2, 0, 6, -11191.1, -1889.4, 107.895
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 2 AND splineId = 0 AND wpId = 6);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 2, 0, 7, -11152.2, -1863.32, 101.006
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 2 AND splineId = 0 AND wpId = 7);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 2, 0, 8, -11130.7, -1891.42, 107.896
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 2 AND splineId = 0 AND wpId = 8);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 2, 0, 9, -11110.7, -1878.77, 107.897
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 2 AND splineId = 0 AND wpId = 9);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 3, 0, 0, -11110.7, -1878.77, 107.897
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 3 AND splineId = 0 AND wpId = 0);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 3, 0, 1, -11142.7, -1891.19, 92.2504
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 3 AND splineId = 0 AND wpId = 1);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 4, 0, 0, -11162.2, -1900.33, 94.7275
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 4 AND splineId = 0 AND wpId = 0);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 4, 0, 1, -11162.2, -1900.33, 91.4726
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 4 AND splineId = 0 AND wpId = 1);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 5, 0, 0, -11160.1, -1870.68, 97.7388
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 5 AND splineId = 0 AND wpId = 0);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 5, 0, 1, -11117.7, -1941.26, 127.172
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 5 AND splineId = 0 AND wpId = 1);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 5, 0, 2, -11123.8, -1968.24, 125.922
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 5 AND splineId = 0 AND wpId = 2);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 5, 0, 3, -11148.3, -1972.98, 116.7
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 5 AND splineId = 0 AND wpId = 3);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 5, 0, 4, -11161.6, -1945.72, 103.089
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 5 AND splineId = 0 AND wpId = 4);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 5, 0, 5, -11168, -1922.91, 97.3941
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 5 AND splineId = 0 AND wpId = 5);
INSERT INTO script_spline_chain_waypoints (entry, chainId, splineId, wpId, x, y, z)
SELECT 17225, 5, 0, 6, -11162.2, -1900.33, 94.7275
WHERE NOT EXISTS (SELECT 1 FROM script_spline_chain_waypoints WHERE entry = 17225 AND chainId = 5 AND splineId = 0 AND wpId = 6);
