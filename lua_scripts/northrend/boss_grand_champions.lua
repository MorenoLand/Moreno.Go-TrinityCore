-- Grand Champions (Trial of the Champion) — Lua port of the JustDied
-- arms of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheChampion/boss_grand_champions.cpp
-- (boss_warrior_toc5, boss_mage_toc5, boss_shaman_toc5,
-- boss_hunter_toc5, boss_rouge_toc5; AddSC_boss_grand_champions at
-- end registers all five via GetTrialOfTheChampionAI).
-- JustDied census (2026-11-07 tick 378): a filename census of the C++
-- tree flagged this file as unported, but entry-ID verification showed
-- most "missing" names were ports under different basenames
-- (boss_anub_arak, boss_krik_thir, boss_epoch, boss_salramm); the
-- grand-champion entries below have zero hits anywhere in Lua or
-- boss_ai.go — genuinely unported.
-- Class -> entry mapping (trial_of_the_champion.h TCCreatureIds,
-- kalecgos pass; class comments in boss_grand_champions.cpp):
--   Warrior: Marshal Jacob Alerius 34705 / Mokra the Skullcrusher 35572
--   Mage: Ambrose Boltspark 34702 / Eressea Dawnsinger 35569
--   Shaman: Colosos 34701 / Runok Wildmane 35571
--   Hunter: Jaelyne Evensong 34657 / Zul'tore 35570
--   Rogue: Lana Stouthammer 34703 / Deathstalker Visceri 35617
-- All five JustDied arms are identical (boss_grand_champions.cpp:436,
-- :580, :732, :893, :1019): instance->SetBossState(BOSS_GRAND_CHAMPIONS,
-- DONE). BOSS_GRAND_CHAMPIONS = 0 (trial_of_the_champion.h TCData),
-- DONE = 3 (InstanceScript.h EncounterState). Bridged via
-- creature:SetBossState (engine/world/lua_creature_events.go:7099).
-- C++-exact note: the instance override
-- (instance_trial_of_the_champion.cpp SetBossState BOSS_GRAND_CHAMPIONS
-- DONE) increments uiGrandChampionsDeaths and spawns the loot chest at
-- 3 — but InstanceScript::SetBossState returns false when the state is
-- already DONE (InstanceScript.cpp:353-360), and Go's
-- setInstanceBossState mirrors that (instance_boss_states.go:183-190),
-- so the override runs only on the FIRST champion death and the count
-- stalls at 1: the loot leg never fires in C++ either. There is also no
-- boss-state-change instance event for Lua to observe the transition
-- (runtime.go InstanceEvents are 1-7 only), so the instance-side
-- counter stays out — faithfully, not as a gap.
-- The combat rotations (UpdateAI timers, mounted-vehicle legs) stay out:
-- this file ports the JustDied census arm only. All six Reset() arms
-- are timer-seed-only for the unported rotations, so ev23 resets
-- nothing in this file's model (round-80 re-verification).
-- The waypoint legs stay out too: SetData cases 1/2/3 (AddWaypoint +
-- Start) and WaypointReached :220/:223 (instance SetData
-- DATA_MOVEMENT_DONE, GetData+1) have no waypoint bridge; the instance
-- consumer (instance_trial_of_the_champion.cpp:210: at 3 the announcer
-- gets SetData(DATA_IN_POSITION, 0)) stays out with them — the
-- announcer escort machine is documented in
-- instance_trial_of_the_champion.lua.

local BOSS_GRAND_CHAMPIONS = 0
local DONE = 3

local ENTRIES = {
    34705, -- Jacob Alerius (warrior)
    35572, -- Mokra the Skullcrusher (warrior)
    34702, -- Ambrose Boltspark (mage)
    35569, -- Eressea Dawnsinger (mage)
    34701, -- Colosos (shaman)
    35571, -- Runok Wildmane (shaman)
    34657, -- Jaelyne Evensong (hunter)
    35570, -- Zul'tore (hunter)
    34703, -- Lana Stouthammer (rogue)
    35617, -- Deathstalker Visceri (rogue)
}

-- C++ JustDied (boss_grand_champions.cpp, all five classes):
-- instance->SetBossState(BOSS_GRAND_CHAMPIONS, DONE).
local function grandChampionDied(event, creature, killer)
    creature:SetBossState(BOSS_GRAND_CHAMPIONS, DONE)
end

for _, entry in ipairs(ENTRIES) do
    RegisterCreatureEvent(entry, 4, grandChampionDied)
end
