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
-- 23 OnReset (the C++ overrides no KilledUnit/JustDied/
-- DamageTaken; the SpellHit transform has no entry-update
-- bridge — no event 14, see deviations).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI in
-- C++ arm order, non-Nihil branch only — the IsNihil speech
-- machine is unreachable behind the unmodeled SpellHit
-- transform, see deviations); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim.
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): per-GUID reset to the C++ ctor /
-- Initialize() values (arcane blast 7500 / mana burn 10000 /
-- intangible presence 15000 — IsNihil is false at creation,
-- the NihilSpeech / IsNihil latch lives only in the
-- unmodeled transform arm) + 1s scheduler pump; the C++
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
-- No talk lines fire in this model: SAY_NIHIL_1..4 and
-- SAY_NIHIL_INTERRUPT all sit in the unmodeled SpellHit /
-- Nihil arms (millhouse SAY_ICEBLOCK precedent).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (the C++ Reset() observable remainder —
-- the Initialize() re-latch — lands on OnEnterCombat,
-- gargolmar precedent; Eluna's On_Reset fires ahead of
-- OnDied and OnSpawn — millhouse note — so both hooks
-- land the same reset).
-- Deliberate deviations (all await engine bridges): no
-- entry-update bridge — the whole SpellHit phase-modulator
-- arm (the entry rotation, me->UpdateEntry, EnterEvadeMode,
-- the Nihil UNIT_FLAG_NON_ATTACKABLE latch, the
-- RemoveFlag interrupt arm, AttackStart on the player
-- caster, Talk(SAY_NIHIL_INTERRUPT)) unmodeled — no event
-- 14 registration; the IsNihil speech machine (SAY_NIHIL_
-- 1..4), the UNIT_FLAG_NOT_SELECTABLE flag arm, the MoveIn
-- LineOfSight NON_ATTACKABLE gate and the MovementInform
-- MovePoint(0)->DespawnOrUnsummon arm all sit behind it or
-- behind flag/movement bridges — unmodeled (netherspite /
-- omor precedents); no summon bridge — the oscillating-
-- frequency scanner master bunny's SummonCreature /
-- SummonGameObject arms unmodeled (steamrigger precedent);
-- no gossip bridge — go_legion_obelisk, go_simon_cluster
-- and go_apexis_relic gossip arms unmodeled; no
-- gameobject-flag/world-search bridges — the simon-game
-- node setup/reset arms (SetCanFly, cluster searches,
-- NearTeleportTo) unmodeled; no SpellScript bridge —
-- spell_oscillating_field documented only (standing
-- blocker).

local SPELL_ARCANE_BLAST = 38881
local SPELL_MANA_BURN = 38884
local SPELL_INTANGIBLE_PRESENCE = 36513

local NETHER_DRAKE_ENTRIES = {
    20021,  -- ENTRY_WHELP
    21821,  -- ENTRY_PROTO
    21817,  -- ENTRY_ADOLE
    21820,  -- ENTRY_MATUR
    21823,  -- ENTRY_NIHIL
}

local drakeState = {}
local drakePump = {}

local function cancelPump(guid)
    local id = drakePump[guid]
    if id then
        RemoveEventById(id)
        drakePump[guid] = nil
    end
end

-- C++ Reset(): the observable remainder — the Initialize()
-- re-latch — lands on OnEnterCombat (gargolmar precedent).
local function fullReset(guid)
    cancelPump(guid)
    drakeState[guid] = nil
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump (it only runs in combat). The IsNihil branch is
-- unreachable behind the unmodeled SpellHit transform, so
-- only the combat arms are ported.
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

for _, entry in ipairs(NETHER_DRAKE_ENTRIES) do
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, onLeaveCombat)
    -- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
    -- this lands the same Reset() reset as the evade hook.
    RegisterCreatureEvent(entry, 23, onLeaveCombat)
end
