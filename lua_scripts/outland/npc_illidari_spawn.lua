-- Illidari Spawn (Shadowmoon Valley) --
-- Lua port of src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (npc_illidari_spawnAI). Zone-script unit per outland_script_loader.cpp
-- order (hellfire_peninsula, nagrand, netherstorm closed; then
-- shadowmoon_valley, terokkar_forest).
-- Entries (verifiable from the C++ sources): the AI branches on
-- me->GetEntry() in UpdateAI and the wave spawner uses the same three
-- ids — 22075 (Illidari Soldier), 22074 (Illidari Mind Breaker),
-- 19797 (Illidari Highlord). The creature_template ScriptName bindings
-- are DB-side (no TDB in this workspace).
-- npc_invis_infernal_caster (invisible summon-caster): the whole AI is
-- the SummonInfernal Reset/SetData machine behind the no-summon bridge
-- plus a display-id check on the summoned infernal — documented-only
-- (steamrigger summon precedent).
-- npc_infernal_attacker (21419, NPC_INFERNAL_ATTACKER — verifiable):
-- Reset = SetDisplayId(MODEL_INVISIBLE) + MoveRandom, SpellHit arm =
-- RemoveFlag(PACIFIED|NOT_SELECTABLE) + SetDisplayId(MODEL_INFERNAL)
-- behind the no-display/flag bridges, the IsSummonedBy GUID latch and
-- JustDied SetData cascade behind the cross-creature SetData bridge —
-- documented-only.
-- npc_mature_netherwing_drake: bridgeable combat arm (Nether Breath
-- 38467 every 5s) but no NPC_ entry constant anywhere in the C++
-- sources — the drake's own entry lives DB-side only — documented-only
-- (standing rule: no invented identifiers).
-- npc_enslaved_netherwing_drake: the Tapped arm sits behind SpellHit
-- (event 14 — no SpellHit bridge) and the faction/move/fly arms behind
-- the no-faction / movement / gravity bridges — documented-only.
-- npc_earthmender_wilda: EscortAI (no escort bridge) + OnQuestAccept
-- (no quest-accept bridge) — documented-only (omor precedent).
-- npc_torloth_the_magnificent (22076, verifiable from WavesInfo): the
-- Torloth cinematic animation machine and the Cleave/Shadowfury/Spell
-- Reflection combat arms sit behind the cross-creature LORDIllidanGUID
-- coordination and the stand-state/anim choreography — documented-only
-- until the event machine bridges exist.
-- npc_lord_illidan_stormrage: the whole event controller (wave summon
-- choreography, LiveCounter/LiveCount bookkeeping, cinematic timers)
-- behind the no-summon / movement / cross-creature bridges —
-- documented-only.
-- go_crystal_prison: GameObjectAI — no gameobject bridge —
-- documented-only.
-- npc_enraged_spirit, spell_unlocking_zuluheds_chains (SpellScript —
-- no SpellScript bridge, standing blocker) and
-- npc_shadowmoon_tuber_node: remaining zone classes — not in this unit.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset (the C++ overrides no KilledUnit/DamageTaken — no events
-- 3/9; no SpellHit override — no event 14).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI in C++ arm
-- order — 1s granularity exact for all C++ timers; the !UpdateVictim
-- early return collapses into the pump; the !Timers first-tick latch
-- ports the C++ per-entry timer init).
-- C++ DoCast default is triggered=false (vaelastrasz
-- convention): creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(target, spell) = DoCast.
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): per-GUID reset to the C++ ctor /
-- Initialize() values (Timers=false, SpellTimer1/2/3=0 —
-- latched on the first combat tick; the C++ JustEngagedWith
-- override is empty, C++-exact) + 1s scheduler pump. No talk
-- lines fire (no SAY_ enums — C++-exact).
-- Pump (C++ UpdateAI arm order, per entry):
-- Illidari Soldier (22075): spellbreaker arm — 10s+0-3s
-- init then 15s+0-4s: non-triggered DoCastVictim(SPELL_
-- SPELLBREAKER 35871) (C++-exact), re-arm regardless.
-- Illidari Mind Breaker (22074): focused-bursts arm —
-- 10s+0-9s init then 10s+0-4s: non-triggered DoCast(random
-- alive player in the instance, SPELL_FOCUSED_BURSTS
-- 38985) (C++-exact — the C++ SelectTarget(Random, 0) is
-- player-gated; the Lua model picks a random alive player
-- in the instance, thespia precedent; nil pick casts
-- nothing and the timer stays expired — C++-exact; the
-- non-player 2000ms re-arm is unreachable in this
-- player-only model), re-arm regardless; psychic-scream
-- arm — 35s+0-3s init then 35s+0-12s: non-triggered
-- DoCastVictim(SPELL_PSYCHIC_SCREAM 22884) (C++-exact),
-- re-arm regardless; mind-blast arm — 20s+0-3s init then
-- 20s+0-7s: non-triggered DoCastVictim(SPELL_MIND_BLAST
-- 17194) (C++-exact), re-arm regardless.
-- Illidari Highlord (19797): curse-of-flames arm —
-- 8s+0-3s init then 15s+0-9s: non-triggered DoCastVictim
-- (SPELL_CURSE_OF_FLAMES 38010) (C++-exact), re-arm
-- regardless; flamestrike arm — 12s+0-3s init then
-- 20s+0-6*13s: non-triggered DoCastVictim(SPELL_
-- FLAMESTRIKE 16102) (C++-exact), re-arm regardless.
-- Melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- OnDied(4): the C++ JustDied arms — me->DespawnOrUnsummon
-- (no despawn bridge — terestian precedent) and the
-- LordIllidan LiveCounter() cross-AI call (no cross-
-- creature bridge) — land nowhere; the hook drops per-GUID
-- state like the other cleanup hooks.
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (the C++ Reset() observable remainder —
-- the Initialize() re-latch — lands on OnEnterCombat,
-- gargolmar precedent; Eluna's On_Reset fires ahead of OnDied
-- and OnSpawn — millhouse note — so both hooks land the
-- same reset).

local ENTRY_ILLIDARI_SOLDIER = 22075
local ENTRY_ILLIDARI_MIND_BREAKER = 22074
local ENTRY_ILLIDARI_HIGHLORD = 19797

local SPELL_SPELLBREAKER = 35871
local SPELL_FOCUSED_BURSTS = 38985
local SPELL_PSYCHIC_SCREAM = 22884
local SPELL_MIND_BLAST = 17194
local SPELL_CURSE_OF_FLAMES = 38010
local SPELL_FLAMESTRIKE = 16102

local spawnState = {}
local spawnPump = {}

local function cancelPump(guid)
    local id = spawnPump[guid]
    if id then
        RemoveEventById(id)
        spawnPump[guid] = nil
    end
end

-- C++ Reset(): the observable remainder — the Initialize()
-- re-latch — lands on OnEnterCombat (gargolmar precedent).
local function fullReset(guid)
    cancelPump(guid)
    spawnState[guid] = nil
end

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ SelectTarget(Random, 0) + player gate: the Lua model
-- picks a random alive player in the instance (thespia
-- precedent); nil pick casts nothing and the timer stays
-- expired (C++-exact).
local function randomPlayer(creature)
    local players = playersInInstance(creature)
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump (it only runs in combat). The !Timers first-tick
-- latch ports the C++ per-entry timer init.
local function combatTick(creature, guid)
    local st = spawnState[guid]
    if not st then
        return
    end

    if not st.timers then
        local entry = creature:GetEntry()
        if entry == ENTRY_ILLIDARI_SOLDIER then
            st.timer1 = 10000 + math.random(0, 3) * 1000
        elseif entry == ENTRY_ILLIDARI_MIND_BREAKER then
            st.timer1 = 10000 + math.random(0, 9) * 1000
            st.timer2 = 35000 + math.random(0, 3) * 1000
            st.timer3 = 20000 + math.random(0, 3) * 1000
        elseif entry == ENTRY_ILLIDARI_HIGHLORD then
            st.timer1 = 8000 + math.random(0, 3) * 1000
            st.timer2 = 12000 + math.random(0, 3) * 1000
        end
        st.timers = true
    end

    local entry = creature:GetEntry()

    -- Illidari Soldier: spellbreaker arm — non-triggered
    -- DoCastVictim, re-arm regardless (C++-exact).
    if entry == ENTRY_ILLIDARI_SOLDIER then
        if st.timer1 <= 1000 then
            creature:CastSpell(nil, SPELL_SPELLBREAKER)
            st.timer1 = 15000 + math.random(0, 4) * 1000
        else
            st.timer1 = st.timer1 - 1000
        end
    end

    -- Illidari Mind Breaker: three arms in C++ order.
    if entry == ENTRY_ILLIDARI_MIND_BREAKER then
        -- Focused bursts: non-triggered DoCast on a random
        -- player; re-arm regardless; nil pick leaves the
        -- timer expired (C++-exact).
        if st.timer1 <= 1000 then
            local target = randomPlayer(creature)
            if target then
                creature:CastSpell(target, SPELL_FOCUSED_BURSTS)
                st.timer1 = 10000 + math.random(0, 4) * 1000
            end
        else
            st.timer1 = st.timer1 - 1000
        end
        -- Psychic scream: non-triggered DoCastVictim,
        -- re-arm regardless (C++-exact).
        if st.timer2 <= 1000 then
            creature:CastSpell(nil, SPELL_PSYCHIC_SCREAM)
            st.timer2 = 35000 + math.random(0, 12) * 1000
        else
            st.timer2 = st.timer2 - 1000
        end
        -- Mind blast: non-triggered DoCastVictim,
        -- re-arm regardless (C++-exact).
        if st.timer3 <= 1000 then
            creature:CastSpell(nil, SPELL_MIND_BLAST)
            st.timer3 = 20000 + math.random(0, 7) * 1000
        else
            st.timer3 = st.timer3 - 1000
        end
    end

    -- Illidari Highlord: two arms in C++ order.
    if entry == ENTRY_ILLIDARI_HIGHLORD then
        -- Curse of flames: non-triggered DoCastVictim,
        -- re-arm regardless (C++-exact).
        if st.timer1 <= 1000 then
            creature:CastSpell(nil, SPELL_CURSE_OF_FLAMES)
            st.timer1 = 15000 + math.random(0, 9) * 1000
        else
            st.timer1 = st.timer1 - 1000
        end
        -- Flamestrike: non-triggered DoCastVictim,
        -- re-arm regardless (C++-exact).
        if st.timer2 <= 1000 then
            creature:CastSpell(nil, SPELL_FLAMESTRIKE)
            st.timer2 = 20000 + math.random(0, 6) * 13000
        else
            st.timer2 = st.timer2 - 1000
        end
    end
end

-- C++ ctor / Initialize() values land on OnEnterCombat:
-- Timers=false, SpellTimer1/2/3=0 — latched on the first
-- combat tick. The C++ JustEngagedWith override is empty —
-- no talk, C++-exact.
local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    spawnState[guid] = {
        timers = false,
        timer1 = 0,
        timer2 = 0,
        timer3 = 0,
    }
    spawnPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end

-- C++ Reset() (called on evade): fullReset — see above.
local function onLeaveCombat(_, creature)
    fullReset(creature:GetGUID())
end

-- C++ JustDied: the DespawnOrUnsummon and the LordIllidan
-- LiveCounter() cross-AI call have no bridges (see header);
-- the hook drops per-GUID state like the other cleanup hooks.
local function onDied(_, creature)
    fullReset(creature:GetGUID())
end

-- The C++ CreatureScript binds one AI class to all three
-- entries (the UpdateAI switches on me->GetEntry()), so all
-- three register the same handlers — C++-exact.
for _, entry in ipairs({
    ENTRY_ILLIDARI_SOLDIER,
    ENTRY_ILLIDARI_MIND_BREAKER,
    ENTRY_ILLIDARI_HIGHLORD,
}) do
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, onLeaveCombat)
    RegisterCreatureEvent(entry, 4, onDied)
    -- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
    -- this lands the same Reset() reset as the evade hook.
    RegisterCreatureEvent(entry, 23, onLeaveCombat)
end
