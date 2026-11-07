-- Nether Drake (Blade's Edge Mountains) --
-- Lua port of src/server/scripts/Outland/zone_blades_edge_
-- mountains.cpp (npc_nether_drakeAI). First zone-script unit
-- after the world-boss set per outland_script_loader.cpp
-- order (AddSC_blades_edge_mountains precedes
-- AddSC_boss_doomlordkazzak).
-- Entries (ENTRY_* constants, verifiable from the C++
-- sources): 20021 (whelp), 21821 (proto), 21817
-- (adolescent), 21820 (mature), 21823 (nihil). The
-- creature_template ScriptName bindings are DB-side (no TDB
-- in this workspace).
-- boss_doomlord_kazzak.cpp and boss_doomwalker.cpp were
-- examined this run: neither carries an NPC_ entry constant
-- anywhere in the C++ sources (entries are DB-side only) —
-- documented-only, unregistered (void_reaver precedent).
-- go_legion_obelisk, npc_simon_bunny, go_simon_cluster,
-- go_apexis_relic, npc_oscillating_frequency_scanner_master_
-- bunny and the spell_oscillating_field SpellScript are
-- documented-only (see deviations).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 23 OnReset, 14 OnHitBySpell, 27 OnMoveInLineOfSight (the C++
-- overrides no KilledUnit/JustDied/DamageTaken; the SpellHit
-- phase-modulator rotation is fully ported — only the
-- MovementInform arrival-despawn leg stays out, on the missing
-- event-28 bridge).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI in
-- C++ arm order — the IsNihil branch runs as its own 1s speech
-- pump, see below); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim.
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): per-GUID reset to the C++ ctor /
-- Initialize() values (arcane blast 7500 / mana burn 10000 /
-- intangible presence 15000 — IsNihil is false at creation,
-- NihilSpeech timer 3000 / phase 0) + 1s scheduler pump; the C++
-- JustEngagedWith override is empty — no pull talk,
-- C++-exact.
-- Pump (C++ UpdateAI arm order): intangible presence arm —
-- 15000 init then 15000+rand()%15000: non-triggered
-- DoCastVictim(SPELL_INTANGIBLE_PRESENCE 36513) (C++-exact),
-- re-arm regardless (C++-exact); mana burn arm — 10000
-- init then 8000+rand()%8000: target = GetVictim(); if the
-- victim's power type is mana (moroes precedent): non-
-- triggered DoCast(target, SPELL_MANA_BURN 38884)
-- (C++-exact), re-arm regardless (C++-exact); arcane blast
-- arm — 7500 init then 2500+rand()%5000: non-triggered
-- DoCastVictim(SPELL_ARCANE_BLAST 38881) (C++-exact),
-- re-arm regardless (C++-exact).
-- No talk lines fire from the combat pump: SAY_NIHIL_1..4 fire
-- from the IsNihil speech pump below (millhouse SAY_ICEBLOCK
-- precedent); SAY_NIHIL_INTERRUPT fires on the NIHIL-source
-- SpellHit leg below.
-- OnLeaveCombat(2)/OnReset(23): cancel the pumps, drop
-- per-GUID state (the C++ Reset() observable remainder —
-- the Initialize() re-latch — lands on OnEnterCombat,
-- gargolmar precedent; Eluna's On_Reset fires ahead of
-- OnDied and OnSpawn — millhouse note — so both hooks
-- land the same reset).
-- MoveInLineOfSight (event 27): the C++ override gates on the
-- NON_ATTACKABLE flag (zone_blades_edge_mountains.cpp:104-110)
-- — a veto while the flag is set, which is exactly the NIHIL
-- state (the flag is set on the NIHIL-destination transform
-- and cleared on the NIHIL-source one); otherwise normal
-- sight-aggro.
-- SpellHit phase-modulator arm PORTED (event 14, C++-exact
-- except the MovementInform delta below): player-caster
-- gate (GetObjectType() == "Player"), cid = rand32() % (4-1)
-- with the bump-if-equal rule, NIHIL-source Talk(SAY_NIHIL_
-- INTERRUPT) + RemoveFlag(NON_ATTACKABLE), then UpdateEntry
-- (new) and EngageWithTarget(player) as the AttackStart twin.
-- NIHIL-destination: UpdateEntry(NIHIL), then EnterEvadeMode
-- (bridge live), SetFlag(NON_ATTACKABLE), IsNihil latch, and
-- the speech machine — SAY_NIHIL_1..4 on the 3000ms-init /
-- 5000ms-re-arm timer (Initialize() value, C++-exact), then
-- phase 4 sets NOT_SELECTABLE and flies off +50x/+50z via
-- MoveTo(0, ...).
-- DELTA: the MovementInform arrival leg (point 0 ->
-- DespawnOrUnsummon(1ms), zone_blades_edge_mountains.cpp:121-
-- 125) stays out — Go never registered the MovementInform
-- hook (no event 28 in scripting/runtime.go), so no hook can
-- observe the landing; the drake flies off and stays at the
-- fly-off point instead of despawning.
-- bunny's SummonCreature / SummonGameObject arms stay out — the
-- unit is unregistered (no entry constant in the C++ sources) and its
-- IsSummonedBy player-capture has no bridge (event 19 fires on the
-- summoner, and the summoner here is a player — no player-side summon
-- hook); the GetClosestCreatureWithEntry leg IS bridged now
-- (creature:FindNearestCreature, lua_creature_events.go:2114), and the
-- summon bridges themselves exist — so only the entry + capture legs
-- block a future bunny port, and the quest-completion SpellScript
-- (no quest-status/complete bridge) blocks the quest itself;
-- stale 'no gossip bridge' retracted (RegisterGameObjectGossipEvent
-- bridged, engine/world/gameobject_hooks.go); go_legion_obelisk,
-- go_simon_cluster
-- and go_apexis_relic gossip arms unmodeled; the simon-game
-- node setup/reset arms (SetCanFly, cluster searches,
-- NearTeleportTo) stay out — the whole game is unreachable
-- behind the missing GO-gossip -> bunny SetGUID/SetData dispatch
-- (SetCanFly / FindNearestGameObject / Teleport / PlayDistanceSound
-- bridges exist now; NearTeleportTo does not, Teleport is its twin); no SpellScript bridge —
-- spell_oscillating_field documented only (standing
-- blocker).

local SPELL_ARCANE_BLAST = 38881
local SPELL_MANA_BURN = 38884
local SPELL_INTANGIBLE_PRESENCE = 36513
local SPELL_T_PHASE_MODULATOR = 37573

local ENTRY_NIHIL = 21823

local SAY_NIHIL_1 = 0
local SAY_NIHIL_2 = 1
local SAY_NIHIL_3 = 2
local SAY_NIHIL_4 = 3
local SAY_NIHIL_INTERRUPT = 4

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local POWER_MANA = 0

local NETHER_DRAKE_ENTRIES = {
    20021,  -- ENTRY_WHELP
    21821,  -- ENTRY_PROTO
    21817,  -- ENTRY_ADOLE
    21820,  -- ENTRY_MATUR
    21823,  -- ENTRY_NIHIL
}

local drakeState = {}
local drakePump = {}
local nihilPump = {}

local function cancelPump(guid)
    local id = drakePump[guid]
    if id then
        RemoveEventById(id)
        drakePump[guid] = nil
    end
end

local function cancelNihilPump(guid)
    local id = nihilPump[guid]
    if id then
        RemoveEventById(id)
        nihilPump[guid] = nil
    end
end

-- C++ Reset(): the observable remainder — the Initialize()
-- re-latch — lands on OnEnterCombat (gargolmar precedent).
-- Both pumps die here: the combat pump and the IsNihil speech
-- pump (a leash evade while NIHIL resets IsNihil, C++-exact).
local function fullReset(guid)
    cancelPump(guid)
    cancelNihilPump(guid)
    drakeState[guid] = nil
end

-- C++ UpdateAI IsNihil branch (zone_blades_edge_mountains.cpp:
-- 166-192): 3000ms init (Initialize() value) then 5000ms
-- re-arms; phases 0-3 Talk SAY_NIHIL_1..4, phase 4 sets
-- NOT_SELECTABLE and flies off +50x/+50z (MovePoint 0);
-- phase 5+ is a no-op switch fall-through, so the pump stops
-- after the fly-off — no observable difference. 1s granularity
-- exact for the timer.
local function nihilTick(creature, guid)
    local st = drakeState[guid]
    if not st or not st.isNihil then
        return
    end

    if st.nihilTimer <= 1000 then
        if st.nihilPhase == 0 then
            creature:Talk(SAY_NIHIL_1)
        elseif st.nihilPhase == 1 then
            creature:Talk(SAY_NIHIL_2)
        elseif st.nihilPhase == 2 then
            creature:Talk(SAY_NIHIL_3)
        elseif st.nihilPhase == 3 then
            creature:Talk(SAY_NIHIL_4)
        elseif st.nihilPhase == 4 then
            creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:MoveTo(0, creature:GetX() + 50.0, creature:GetY(), creature:GetZ() + 50.0)
            cancelNihilPump(guid)
            return
        end
        st.nihilPhase = st.nihilPhase + 1
        st.nihilTimer = 5000
    else
        st.nihilTimer = st.nihilTimer - 1000
    end
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump (it only runs in combat). The IsNihil branch runs
-- on its own speech pump (nihilTick below) — the combat arms
-- never run while IsNihil, C++-exact (the UpdateAI early
-- return).
local function combatTick(creature, guid)
    local st = drakeState[guid]
    if not st then
        return
    end

    -- Intangible presence arm — 15000 init then
    -- 15000+rand()%15000: non-triggered DoCastVictim,
    -- re-arm regardless (C++-exact).
    if st.intangiblePresenceTimer <= 1000 then
        creature:CastSpell(nil, SPELL_INTANGIBLE_PRESENCE)
        st.intangiblePresenceTimer = 15000 + math.random(0, 14999)
    else
        st.intangiblePresenceTimer = st.intangiblePresenceTimer - 1000
    end

    -- Mana burn arm — 10000 init then 8000+rand()%8000:
    -- cast only when the victim's power type is mana
    -- (moroes precedent), re-arm regardless (C++-exact).
    if st.manaBurnTimer <= 1000 then
        local victim = creature:GetVictim()
        if victim and victim:GetPowerType() == POWER_MANA then
            creature:CastSpell(victim, SPELL_MANA_BURN)
        end
        st.manaBurnTimer = 8000 + math.random(0, 7999)
    else
        st.manaBurnTimer = st.manaBurnTimer - 1000
    end

    -- Arcane blast arm — 7500 init then 2500+rand()%5000:
    -- non-triggered DoCastVictim, re-arm regardless
    -- (C++-exact).
    if st.arcaneBlastTimer <= 1000 then
        creature:CastSpell(nil, SPELL_ARCANE_BLAST)
        st.arcaneBlastTimer = 2500 + math.random(0, 4999)
    else
        st.arcaneBlastTimer = st.arcaneBlastTimer - 1000
    end
end

-- C++ ctor / Initialize() values land on OnEnterCombat:
-- arcane blast 7500 / mana burn 10000 / intangible presence
-- 15000 (IsNihil false at creation). The C++ JustEngagedWith
-- override is empty — no talk, C++-exact.
local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    -- Entering combat means not NIHIL (C++ UpdateAI runs the
    -- IsNihil branch or the combat branch, never both) — a
    -- stray speech pump dies here if one survived.
    cancelNihilPump(guid)
    drakeState[guid] = {
        arcaneBlastTimer = 7500,
        manaBurnTimer = 10000,
        intangiblePresenceTimer = 15000,
    }
    drakePump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end

-- C++ Reset() (called on evade): fullReset — see above.
local function onLeaveCombat(_, creature)
    fullReset(creature:GetGUID())
end

-- C++ MoveInLineOfSight (zone_blades_edge_mountains.cpp:104-
-- 110): no sight-aggro while NON_ATTACKABLE is set — exactly
-- the NIHIL state. An ev27 veto while the flag is set, normal
-- sight-aggro otherwise.
local function onMoveInLineOfSight(_, creature)
    if creature:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE) then
        return true
    end
    return false
end

-- C++ entry rotation order: PROTO, ADOLE, MATUR, NIHIL.
local PHASE_ENTRY_LIST = {
    21821,  -- ENTRY_PROTO
    21817,  -- ENTRY_ADOLE
    21820,  -- ENTRY_MATUR
    21823,  -- ENTRY_NIHIL
}

-- C++ SpellHit (zone_blades_edge_mountains.cpp:128-160) in
-- arm order. The combat pump above is per-GUID and entry-
-- independent, so it survives the transform exactly as the
-- C++ AI object does (no Reset runs on UpdateEntry).
local function onSpellHit(_, creature, caster, spellId)
    if spellId ~= SPELL_T_PHASE_MODULATOR then
        return
    end
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end

    local guid = creature:GetGUID()
    local current = creature:GetEntry()
    local cid = math.random(0, 2)
    if PHASE_ENTRY_LIST[cid + 1] == current then
        cid = cid + 1
    end

    if current == ENTRY_NIHIL then
        creature:Talk(SAY_NIHIL_INTERRUPT)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        -- Leave the speech machine: cancel its pump and drop
        -- the IsNihil state. The combat pump restarts at
        -- Initialize values on the EngageWithTarget below (its
        -- ev1 hook runs onEnterCombat) — a documented delta:
        -- C++ resumes the frozen combat timers, Lua restarts
        -- them, because the evade cancelled the pump.
        cancelNihilPump(guid)
        drakeState[guid] = nil
    end

    local newEntry = PHASE_ENTRY_LIST[cid + 1]
    if newEntry == ENTRY_NIHIL then
        -- C++ order: UpdateEntry, then EnterEvadeMode, then
        -- SetFlag(NON_ATTACKABLE), IsNihil = true. The evade
        -- fires ev23/ev2 -> fullReset (combat pump cancelled,
        -- state dropped), so the IsNihil state + speech pump
        -- are latched after it.
        if creature:UpdateEntry(newEntry) then
            creature:EnterEvadeMode()
            creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
            drakeState[guid] = {
                isNihil = true,
                nihilPhase = 0,
                nihilTimer = 3000,
            }
            nihilPump[guid] = CreateLuaEvent(function()
                nihilTick(creature, guid)
            end, 1000, 0)
        end
        return
    end
    if creature:UpdateEntry(newEntry) then
        creature:EngageWithTarget(caster)
    end
end

for _, entry in ipairs(NETHER_DRAKE_ENTRIES) do
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, onLeaveCombat)
    -- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
    -- this lands the same Reset() reset as the evade hook.
    RegisterCreatureEvent(entry, 23, onLeaveCombat)
    RegisterCreatureEvent(entry, 14, onSpellHit)
    RegisterCreatureEvent(entry, 27, onMoveInLineOfSight)
end
