-- Warchief Kargath Bladefist (Shattered Halls, Hellfire
-- Citadel) — Lua port of src/server/scripts/Outland/
-- HellfireCitadel/ShatteredHalls/boss_warchief_kargath_
-- bladefist.cpp (boss_warchief_kargath_bladefist — the only
-- creature AI class AddSC registers; GetShatteredHallsAI is
-- an AI-factory helper, not a script).
-- Final boss in the Shattered Halls set per
-- outland_script_loader.cpp order (instance_shattered_halls.
-- cpp stays blocked on the instance-script model).
-- Entry: NPC_KARGATH_BLADEFIST = 16808 from shattered_halls.
-- h (SHCreatureIds), mapped in instance_shattered_halls.cpp
-- OnCreatureCreate (line 111) plus GetGuidData (line 170).
-- The creature_template ScriptName bindings are DB-side (no
-- TDB in this workspace).
-- Talk: SAY_AGGRO = 0 fires from the C++ JustEngagedWith
-- (C++-exact); SAY_SLAY = 1 fires from the C++ KilledUnit
-- override gated on the victim being a player (C++-exact);
-- SAY_DEATH = 2 fires from the C++ JustDied (the _JustDied
-- arm is instance-blocked); SAY_CALL_EXECUTIONER_A = 3 /
-- SAY_CALL_EXECUTIONER_H = 4 fire only from the DoAction
-- (ACTION_EXECUTIONER_TAUNT = 1, shattered_halls.h) arm,
-- which reads instance->GetData(DATA_TEAM_IN_INSTANCE) —
-- instance gated, unreachable in this model.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied,
-- 4 OnDied. A scheduler pump exists for the Assassins_Timer arm
-- only (see below); every other combat timer's fire arm is
-- blocked on a missing bridge, so nothing else observable remains
-- to tick; melee is engine-driven in Go (creature combat tick),
-- like C++.
-- Fight shape (C++-exact for all modeled arms): Kargath
-- (16808): OnEnterCombat(1) — Talk(SAY_AGGRO); the C++
-- JustEngagedWith override does nothing else, and the
-- Initialize() schedule (Blade_Dance_Timer 45000 /
-- Summon_Assistant_Timer 30000 / Assassins_Timer 5000 /
-- resetcheck_timer 5000 / summoned 2) drives bridge-blocked arms
-- except Assassins_Timer, so per-GUID state is kept only for the
-- assassin pump (fired flag + stored summon objects).
-- OnTargetDied(3): Talk(SAY_SLAY) gated on the killed unit
-- being a player (C++ `victim->GetTypeId() == TYPEID_PLAYER`
-- via the terestian/gurtogg 3-arg handler, C++-exact).
-- OnDied(4): Talk(SAY_DEATH) (the _JustDied arm is
-- instance-blocked; the removeAdds assassin sweep is PORTED via
-- stored summon objects — the guard list stays empty because
-- guards are never summoned, header). OnReset(23): the C++ Reset()
-- SetSpeedRate(MOVE_RUN, 2) arm, ported via creature:SetSpeedRate
-- (the SetWalk(false) leg has no bridge; removeAdds/_Reset/
-- Initialize have no observable remainder in this model, omrogg
-- "no event 3" precedent). Melee is
-- engine-driven.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_shattered_halls.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_KARGATH = 2
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped —
-- DATA_KARGATH is an instance-side constant, unbridgeable);
-- the Assassins_Timer 5s arm (four NPC_SHATTERED_ASSASSIN
-- 17695 summons at AssassEntrance/AssassExit with
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT) is PORTED 2026-10-24
-- via a 5s one-shot pump — the stale "no summon bridge" claim
-- is retracted (SummonCreature is bridged; C++ JustSummoned
-- for assassins only tracks GUIDs, no AttackStart relay to
-- miss). The Summon_Assistant_Timer {25s,35s} arm (summoned
-- NPC_HEARTHEN_GUARD 17621 / NPC_SHARPSHOOTER_GUARD 17622 /
-- NPC_REAVER_GUARD 17623 at AddsEntrance, with the
-- summoned-counter growth arm) stays out: C++ JustSummoned
-- AttackStarts each guard onto a random target (no bridge),
-- so summoning guards would strand them off-target
-- (steamrigger precedent); no movement bridge — the whole
-- blade-dance machine unmodeled: Blade_Dance_Timer 45s init
-- then 30s sets target_num = 5 / Wait_Timer = 1 / InBlade =
-- true / SetSpeedRate(MOVE_RUN, 4), the Wait_Timer arm drives
-- MovePoint(1) hops to random points around (210, -60) and
-- MovementInform(POINT_MOTION_TYPE, 1) fires the triggered
-- DoCast(me, SPELL_BLADE_DANCE 30739, true) with target_num--
-- until target_num hits 0, then InBlade = false /
-- SetSpeedRate(MOVE_RUN, 2) / MoveChase(victim) /
-- Blade_Dance_Timer = 30000 — none of it can run without a
-- movement bridge; the speed-rate arms (SetSpeedRate MOVE_RUN is
-- bridged, lua_creature_events.go:735) stay out with the unbridged
-- MovePoint/MovementInform machine; the IsHeroic()-gated Charge_timer = 5000 arm at
-- blade-dance end and its DoCast(SelectTarget(Random, 0),
-- H_SPELL_CHARGE 25821) fire arm stay unmodeled — the "no
-- difficulty bridge" claim was stale and is retracted
-- (creature:IsHeroic() is bridged, lua_creature_events.go:260),
-- but the arm still has no landing point: Charge_timer is only
-- ever armed by the blade-dance-end leg (no movement bridge —
-- MovePoint/MovementInform) and the fire arm needs
-- SelectTarget(Random, 0) non-playerOnly, which has no bridge
-- (kagani-vanish precedent); porting the gate alone would
-- fabricate a fire arm with no trigger (thespia precedent); no cross-creature/
-- ObjectAccessor bridge — the JustSummoned AttackStart relay
-- for guard adds stays unmodeled; the removeAdds sweep is
-- ported for assassins via stored summon objects (guards never
-- summoned, so their list is empty by construction); no position/home bridge — the resetcheck_timer
-- 5s evade arm (EnterEvadeMode when x is outside [205, 255])
-- unmodeled (bookkeeping exact: 5s init then 5s); no
-- unit-state bridge — the C++ UpdateAI skips
-- DoMeleeAttackIfReady entirely while InBlade (no melee
-- during blade dance) and there is no bridge to suppress
-- engine melee; the ScriptData "SD%Complete: 90" is an
-- upstream caveat, documented, not bridged; no SpellScript/
-- AuraScript scripts in this file.

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local ENTRY_KARGATH_BLADEFIST = 16808
local ENTRY_SHATTERED_ASSASSIN = 17695
-- C++ AssassEntrance/AssassExit (boss_warchief_kargath_bladefist.cpp:62-63);
-- SpawnAssassin (:195-198) summons at y +/- 8, orientation 0,
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT (bridge type 2), 30s.
local ASSASSIN_SPOTS = {
    {275.136, -84.29 + 8, 2.3},
    {275.136, -84.29 - 8, 2.3},
    {184.233, -84.29 + 8, 2.3},
    {184.233, -84.29 - 8, 2.3},
}

local kargathTimers = {}
local kargathAssassins = {}
local kargathAssassinFired = {}

local function cancelKargath(guid)
    local id = kargathTimers[guid]
    if id then
        RemoveEventById(id)
        kargathTimers[guid] = nil
    end
    kargathAssassinFired[guid] = nil
end

-- C++ removeAdds (:175-191) for the assassin list: the guard list
-- stays empty in this model (guards are never summoned — header),
-- so only the assassin list is swept.
local function despawnAssassins(guid)
    local list = kargathAssassins[guid]
    if list then
        for _, add in ipairs(list) do
            if add then
                add:Despawn()
            end
        end
        kargathAssassins[guid] = nil
    end
end

local function kargathEnterCombat(event, creature)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    kargathAssassinFired[guid] = false
    kargathAssassins[guid] = {}
    -- C++ Assassins_Timer 5000 one-shot (UpdateAI :207-213):
    -- SpawnAssassin() fires once, then Assassins_Timer = 0.
    -- The 5s pump re-fires but is fired-guarded; canceled on
    -- events 2/4/23. C++ JustSummoned for assassins (:139-141)
    -- only tracks GUIDs (no AttackStart relay), so the summon
    -- arm is complete without a cross-creature bridge.
    kargathTimers[guid] = CreateLuaEvent(function()
        if kargathAssassinFired[guid] then
            return
        end
        kargathAssassinFired[guid] = true
        local list = kargathAssassins[guid] or {}
        for _, spot in ipairs(ASSASSIN_SPOTS) do
            local add = creature:SummonCreature(ENTRY_SHATTERED_ASSASSIN, spot[1], spot[2], spot[3], 0, 2, 30000)
            if add then
                list[#list + 1] = add
            end
        end
        kargathAssassins[guid] = list
    end, 5000)
end

local function kargathLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelKargath(guid)
    despawnAssassins(guid)
end

local function kargathDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    local guid = creature:GetGUID()
    cancelKargath(guid)
    despawnAssassins(guid)
end

local function kargathReset(event, creature)
    -- C++ Reset() :114: SetSpeedRate(MOVE_RUN, 2); the SetWalk(false) leg
    -- has no bridge, and the rest of Reset (removeAdds/_Reset/Initialize)
    -- has no observable remainder in this model except the assassin
    -- sweep above (documented above).
    creature:SetSpeedRate(1, 2)
    local guid = creature:GetGUID()
    cancelKargath(guid)
    despawnAssassins(guid)
end

RegisterCreatureEvent(ENTRY_KARGATH_BLADEFIST, 1, kargathEnterCombat)

RegisterCreatureEvent(ENTRY_KARGATH_BLADEFIST, 3, function(_, creature, victim)
    -- C++ `if (victim->GetTypeId() == TYPEID_PLAYER)
    -- Talk(SAY_SLAY)` — terestian/gurtogg 3-arg-handler
    -- convention (C++-exact).
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)

RegisterCreatureEvent(ENTRY_KARGATH_BLADEFIST, 4, kargathDied)

RegisterCreatureEvent(ENTRY_KARGATH_BLADEFIST, 23, kargathReset)
