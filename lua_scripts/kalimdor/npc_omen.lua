-- Omen (Moonglade) --
-- Lua port of src/server/scripts/Kalimdor/zone_moonglade.cpp
-- (npc_omenAI — combat rotation + JustDied spotlight summon +
-- Elune's Candle SpellHit starfall-reschedule arm only). Zone-
-- script unit per kalimdor_script_loader.cpp order (felwood,
-- feralas done; moonglade: npc_clintar_spirit documented-only,
-- npc_omen ported, npc_giant_spotlight documented-only).
-- Entry (Omen enum, verifiable from the C++ sources): 15467
-- (NPC_OMEN). The creature_template ScriptName binding is
-- DB-side (no TDB in this workspace).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 14 OnSpellHit (the C++ overrides no
-- MovementInform-bridgeable arms, no KilledUnit/
-- DamageTaken — no events 3/9).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI's
-- combat section in C++ arm order — 1s granularity exact
-- for all C++ timers; the !UpdateVictim early return
-- collapses into the pump, which only runs in combat).
-- C++ DoCast default is triggered=false (vaelastrasz
-- convention): creature:CastSpell(nil, spell) = DoCastVictim.
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): per-GUID reset to the C++ JustEngagedWith
-- schedule values (cleave 3-5s, starfall 8-10s) + 1s scheduler
-- pump. Each pump tick decrements every timer by 1000 (the
-- C++ EventMap::Update(diff) pass), then expired events fire
-- in C++ arm order (a single ExecuteEvent returns the first
-- expired event per UpdateAI call; at 1s pump granularity
-- simultaneous expiries land in the same tick in C++ switch
-- order — cleave before starfall).
-- Pump (C++ UpdateAI arm order): cleave arm — non-triggered
-- DoCastVictim(SPELL_OMEN_CLEAVE 15284) (C++-exact), re-arm
-- 8-10s (no DelayEvents in the C++ UpdateAI, C++-exact);
-- starfall arm — non-triggered DoCast(SPELL_OMEN_STARFALL
-- 26540) (C++-exact), re-arm 14-16s. Melee is engine-driven.
-- OnDied(4): non-triggered DoCast(self, SPELL_OMEN_SUMMON_
-- SPOTLIGHT 26392) (the C++ JustDied arm, C++-exact) + drop
-- per-GUID state like the other cleanup hooks.
-- OnSpellHit(14): SPELL_ELUNE_CANDLE 26374 -> the C++
-- RescheduleEvent(EVENT_CAST_STARFALL, 14s, 16s) arm is
-- modeled as re-arming the starfall timer to 14-16s
-- (C++-exact); the aura arm is a documented deviation below.
-- The event-14 caster is always the player proxy in this
-- model (desolace-kodo precedent) and the C++ arm gates only
-- on the spell id, so no caster gate is needed (C++-exact).
-- OnLeaveCombat(2): cancel the pump, drop per-GUID state
-- (the C++ event-map reset lands on OnEnterCombat,
-- gargolmar precedent).
-- Deliberate deviations (all await engine bridges): the
-- constructor's SetImmuneToPC(true) + MovePoint to
-- (7549.977, -2855.137, 456.9678) has no immune / movement
-- bridges — unmodeled; MovementInform point 1 (SetHomePosition
-- + SetImmuneToPC(false) + SelectNearestPlayer(40) ->
-- AttackStart) has no movement / world-search bridges —
-- unmodeled; the starfall arm's SelectTarget(Random, 0) rides
-- the GetAITargets threat-list pool (the 02:4x sweep7 fold-in;
-- the old "no target-selection bridge" claim was stale); the Elune's Candle
-- aura arm (HasAura(SPELL_OMEN_STARFALL) ->
-- RemoveAurasDueToSpell) is ported via the bridged
-- creature:HasAura/creature:RemoveAura (lua_objects.go:228,242
-- — hakkar/netherspite precedent), C++-exact on event 14;
-- the re-arm still fires on every 26374 hit, C++-exact for
-- the reschedule half; the C++ constructor-
-- driven immunity phase means omen is attackable from spawn
-- in this model.
-- npc_clintar_spirit — the whole quest-10965 escort event
-- machine (41-waypoint AddWaypoint/Start event, IsSummonedBy
-- player-search latch, JustDied FailQuest, waypoint Talk /
-- emotestate / SummonCreature(ASPECT_RAVEN 22915) /
-- CompleteQuest arms) is behind the no-escort / no-quest-
-- accept / no-talk / no-summon / no-quest / no-cross-creature
-- bridges — documented-only.
-- npc_giant_spotlight — the 5-minute EVENT_DESPAWN machine
-- (FindNearestGameObject(GO_ELUNE_TRAP_1 180876 /
-- GO_ELUNE_TRAP_2 180877) -> RemoveFromWorld +
-- FindNearestCreature(NPC_OMEN 15467) -> DespawnOrUnsummon):
-- PORTED in lua_scripts/kalimdor/npc_giant_spotlight.lua —
-- entry 15902 Wowhead-verified (spell 26392 Effect #1 summons
-- Giant Spotlight); registered in boss_ai.go.
-- Zone set status: zone_moonglade.cpp closed (npc_clintar_
-- spirit — quest-10965 escort machine documented-only;
-- npc_giant_spotlight — 5-min despawn sweep ported;
-- npc_omen combat rotation ported for 15467).

local SPELL_OMEN_CLEAVE = 15284
local SPELL_OMEN_STARFALL = 26540
local SPELL_OMEN_SUMMON_SPOTLIGHT = 26392
local SPELL_ELUNE_CANDLE = 26374

local OMEN_ENTRY = 15467

local omenState = {}
local omenPump = {}

local TIMER_KEYS = { "cleaveTimer", "starfallTimer" }

local function cancelPump(guid)
    local id = omenPump[guid]
    if id then
        RemoveEventById(id)
        omenPump[guid] = nil
    end
end

local function fullReset(guid)
    cancelPump(guid)
    omenState[guid] = nil
end

-- C++ UpdateAI combat section in arm order — 1s granularity
-- exact for all C++ timers; the !UpdateVictim early return
-- collapses into the pump (it only runs in combat). Each
-- tick decrements every timer (the EventMap::Update(diff)
-- pass), then expired events fire in C++ arm order.
local function combatTick(creature, guid)
    local st = omenState[guid]
    if not st then
        return
    end

    for _, key in ipairs(TIMER_KEYS) do
        st[key] = st[key] - 1000
    end

    -- Cleave arm — non-triggered DoCastVictim, re-arm 8-10s
    -- (no DelayEvents in the C++ UpdateAI, C++-exact).
    if st.cleaveTimer <= 0 then
        creature:CastSpell(nil, SPELL_OMEN_CLEAVE)
        st.cleaveTimer = 8000 + math.random(0, 2000)
    end

    -- Starfall arm — SelectTarget(Random, 0) (C++ defaults:
    -- playerOnly=false) over the threat list ->
    -- creature:GetAITargets() alive pool (C++-exact pool; the
    -- jaraxxus idiom without its player filter); nil-guarded
    -- pick (C++ `if` — skip cast on nil); DoCast 2-arg =
    -- non-triggered -> CastSpell(target, spell); re-arm 14-16s
    -- unconditional (outside the target-if, C++-exact).
    if st.starfallTimer <= 0 then
        local candidates = {}
        for _, unit in ipairs(creature:GetAITargets()) do
            if not unit:IsDead() then
                candidates[#candidates + 1] = unit
            end
        end
        if #candidates > 0 then
            creature:CastSpell(candidates[math.random(#candidates)], SPELL_OMEN_STARFALL)
        end
        st.starfallTimer = 14000 + math.random(0, 2000)
    end
end

-- C++ JustEngagedWith schedule values land on
-- OnEnterCombat: cleave 3-5s, starfall 8-10s (events.Reset
-- in the C++ latch — the pump is cancelled first,
-- gargolmar precedent).
local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    omenState[guid] = {
        cleaveTimer = 3000 + math.random(0, 2000),
        starfallTimer = 8000 + math.random(0, 2000),
    }
    omenPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end

-- The C++ JustDied arm: non-triggered DoCast(self,
-- SPELL_OMEN_SUMMON_SPOTLIGHT) — C++-exact. The hook drops
-- per-GUID state like the other cleanup hooks.
local function onDied(_, creature)
    creature:CastSpell(creature, SPELL_OMEN_SUMMON_SPOTLIGHT)
    fullReset(creature:GetGUID())
end

-- C++ SpellHit: SPELL_ELUNE_CANDLE -> RescheduleEvent(EVENT_
-- CAST_STARFALL, 14s, 16s) — modeled as re-arming the
-- starfall timer to 14-16s (C++-exact). The aura-strip half
-- is ported via the bridged creature:HasAura/creature:
-- RemoveAura (lua_objects.go:228,242 — hakkar/netherspite
-- precedent), firing unconditionally on every 26374 hit
-- before the re-arm (C++-exact). The
-- event-14 caster is always the player proxy in this model
-- and the C++ arm gates only on the spell id (desolace-kodo
-- precedent). Out-of-combat hits find no pump state — the
-- next OnEnterCombat re-latches the JustEngagedWith
-- schedule anyway (C++-equivalent: RescheduleEvent on the
-- evade-reset map is superseded by the next JustEngagedWith
-- Reset).
local function onSpellHit(_, creature, _, spellId)
    if spellId ~= SPELL_ELUNE_CANDLE then
        return
    end
    -- C++ SpellHit (zone_moonglade.cpp:454-462): strip the
    -- starfall aura first, unconditionally — C++-exact
    -- ordering.
    if creature:HasAura(SPELL_OMEN_STARFALL) then
        creature:RemoveAura(SPELL_OMEN_STARFALL)
    end
    local st = omenState[creature:GetGUID()]
    if st then
        st.starfallTimer = 14000 + math.random(0, 2000)
    end
end

local function onLeaveCombat(_, creature)
    fullReset(creature:GetGUID())
end

RegisterCreatureEvent(OMEN_ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(OMEN_ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(OMEN_ENTRY, 4, onDied)
RegisterCreatureEvent(OMEN_ENTRY, 14, onSpellHit)
