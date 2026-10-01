-- Enraged Spirits (Shadowmoon Valley) --
-- Lua port of src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (npc_enraged_spiritAI). Zone-script unit after npc_illidari_spawn
-- per outland_script_loader.cpp order (netherstorm,
-- shadowmoon_valley, terokkar_forest).
-- Entries (Enraged_Dpirits enum, verifiable from the C++
-- sources): 21050 (NPC_ENRAGED_EARTH_SPIRIT), 21061
-- (NPC_ENRAGED_FIRE_SPIRIT), 21060 (NPC_ENRAGED_AIR_SPIRIT),
-- 21059 (NPC_ENRAGED_WATER_SPIRIT). The C++ CreatureScript
-- GetAI returns the same AI class for all four entries (the
-- JustEngagedWith/UpdateAI/JustDied switch on GetEntry()),
-- so all four register the same handlers (C++-exact). The
-- creature_template ScriptName bindings are DB-side (no TDB
-- in this workspace). No talk lines fire in this model (no
-- SAY_ enums in the AI — C++-exact).
-- spell_unlocking_zuluheds_chains (NPC_KARYNAKU = 22112) is
-- documented-only: SpellScript, no SpellScript bridge
-- (standing blocker). npc_shadowmoon_tuber_node is
-- documented-only: no entry constant for the tuber node
-- itself anywhere in the C++ sources (only SPELL_WHISTLE
-- 36652, SPELL_SHADOWMOON_TUBER 36462, NPC_BOAR_ENTRY 21195,
-- GO_SHADOWMOON_TUBER_MOUND 184701), and the whole AI sits
-- behind the no-SetData bridge (SetData(TYPE_BOAR,
-- DATA_BOAR) spawn chest arm) and the no-SpellHit /
-- no-movement / no-world-search bridges (SpellHit SPELL_
-- WHISTLE -> FindNearestCreature boar -> MovePoint arm) —
-- standing rule: no invented identifiers; unregistered is
-- dead code (the luaBossAI shim admits only registered
-- entries).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 9 OnDamageTaken (the enrage latch), 23 OnReset. No event
-- 3 — the C++ overrides no KilledUnit; no event 4 — the
-- JustDied arm is unbridgeable (see deviations); no event
-- 14 — no SpellHit override.
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI in
-- C++ arm order — 1s granularity exact for all C++ timers;
-- the !UpdateVictim early return collapses into the pump).
-- C++ DoCast default is triggered=false (vaelastrasz
-- convention): creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(creature, spell) = DoCastSelf.
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): per-GUID reset to the C++ JustEngagedWith
-- values (the C++ Reset() override is empty — the timer
-- re-latch lands on OnEnterCombat, gargolmar precedent).
-- Earth spirit (21050): stormTimer 3000-7000 init (3s+0-4s)
-- then 6000-9000: non-triggered DoCastVictim(SPELL_FIERY_
-- BOULDER 38498) (C++-exact), re-arm 6-9s regardless.
-- Fire spirit (21061): stormTimer 2000-12000 init (2s+0-10s)
-- then 6000-12000: non-triggered DoCastVictim(SPELL_FEL_
-- FIREBALL 36247) (C++-exact), re-arm 6-12s regardless.
-- Air spirit (21060): chainLightningTimer 10000 init:
-- non-triggered DoCastVictim(SPELL_CHAIN_LIGHTNING 12058)
-- (C++-exact), then hurricaneTimer 3000-5000: non-triggered
-- DoCastVictim(SPELL_HURRICANE 32717) (C++-exact), then
-- chainLightningTimer 15000-20000 (C++-exact chain/hurricane
-- alternation). Water spirit (21059): stormTimer 0-1000
-- init (0s+0-1s) then 17000-23000: non-triggered
-- DoCastVictim(SPELL_STORMBOLT 38032) (C++-exact), re-arm
-- 17-23s regardless.
-- OnDamageTaken(9) — the C++ UpdateAI per-tick enrage latch
-- (fire + air only): HealthBelowPctDamaged(35) — health
-- after this hit strictly below 35% (shahraz precedent) and
-- a used_enrage latch (the !GetAura(SPELL_ENRAGE) check has
-- no aura-state bridge — phase_hunter precedent — so the
-- dispel re-cast arm is unmodeled, see deviations):
-- non-triggered DoCastSelf(SPELL_ENRAGE 8599) (C++-exact).
-- Melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (Eluna's On_Reset fires ahead of OnDied and
-- OnSpawn — millhouse note — so both hooks land the same
-- reset).
-- Deliberate deviations (all await engine bridges): no
-- aura-state bridge — the JustEngagedWith !GetAura(SPELL_
-- FEL_FIRE_AURA 36006) gated self-cast (fire + earth) and
-- the enrage latch's !GetAura(SPELL_ENRAGE) re-cast arm
-- unmodeled (phase_hunter precedent); no summon bridge —
-- the JustDied DoSpawnCreature soul arms (NPC_EARTHEN_SOUL
-- 21073 / NPC_FIERY_SOUL 21097 / NPC_ENRAGED_AIRY_SOUL
-- 21116 / NPC_ENRAGED_WATERY_SOUL 21109) and the earth
-- spirit's DoCastSelf(SPELL_SUMMON_ENRAGED_EARTH_SHARD
-- 38365) unmodeled (steamrigger precedent); no world-search
-- / faction / movement / quest-credit bridges — the JustDied
-- FindNearestCreature(ENTRY_TOTEM_OF_SPIRITS 21071, 15yd)
-- arm (SetFaction(FRIENDLY), MovePoint to the totem,
-- KilledMonsterCredit(NPC_CREDIT_*) via the totem's player
-- owner, DoCast(totem, SPELL_SOUL_CAPTURED 36115))
-- unmodeled — so no event 4 is registered.
-- Zone set status: zone_shadowmoon_valley.cpp — the Torloth
-- / Illidan cinematic event machine, the infernal summon
-- pair, the mature/enslaved drake quest arms and the wilda
-- escort remain documented-only behind the no-summon /
-- movement / SpellHit / cross-creature / quest / escort
-- bridges; npc_illidari_spawn ported for 22075/22074/19797;
-- npc_enraged_spirit ported for 21050/21061/21060/21059;
-- spell_unlocking_zuluheds_chains + npc_shadowmoon_tuber_
-- node documented-only. Next per outland_script_loader.cpp
-- order: zone_terokkar_forest.cpp.

local SPELL_STORMBOLT = 38032
local SPELL_FEL_FIREBALL = 36247
local SPELL_FIERY_BOULDER = 38498
local SPELL_CHAIN_LIGHTNING = 12058
local SPELL_HURRICANE = 32717
local SPELL_ENRAGE = 8599

local ENTRY_ENRAGED_EARTH_SPIRIT = 21050
local ENTRY_ENRAGED_FIRE_SPIRIT = 21061
local ENTRY_ENRAGED_AIR_SPIRIT = 21060
local ENTRY_ENRAGED_WATER_SPIRIT = 21059

local spiritState = {}
local spiritPump = {}

local function cancelPump(guid)
    local id = spiritPump[guid]
    if id then
        RemoveEventById(id)
        spiritPump[guid] = nil
    end
end

local function fullReset(guid)
    cancelPump(guid)
    spiritState[guid] = nil
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump (it only runs in combat).
local function combatTick(creature, guid)
    local st = spiritState[guid]
    if not st then
        return
    end

    if st.entry == ENTRY_ENRAGED_AIR_SPIRIT then
        -- Air alternation (C++ arm order): chain lightning
        -- fires -> schedules hurricane 3-5s; hurricane fires
        -- -> schedules chain lightning 15-20s.
        if st.chainLightningTimer then
            if st.chainLightningTimer <= 1000 then
                creature:CastSpell(nil, SPELL_CHAIN_LIGHTNING)
                st.chainLightningTimer = nil
                st.hurricaneTimer = 3000 + math.random(0, 2000)
            else
                st.chainLightningTimer = st.chainLightningTimer - 1000
            end
        end
        if st.hurricaneTimer then
            if st.hurricaneTimer <= 1000 then
                creature:CastSpell(nil, SPELL_HURRICANE)
                st.hurricaneTimer = nil
                st.chainLightningTimer = 15000 + math.random(0, 5000)
            else
                st.hurricaneTimer = st.hurricaneTimer - 1000
            end
        end
    else
        -- Water: 17-23s; fire: 6-12s; earth: 6-9s — re-arm
        -- regardless of victim pick (C++-exact).
        if st.stormTimer <= 1000 then
            creature:CastSpell(nil, st.stormSpell)
            st.stormTimer = st.stormReArm()
        else
            st.stormTimer = st.stormTimer - 1000
        end
    end
end

-- C++ JustEngagedWith (the C++ Reset() override is empty —
-- the timer re-latch lands on OnEnterCombat, gargolmar
-- precedent).
local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    local entry = creature:GetEntry()
    local st = { entry = entry, usedEnrage = false }
    if entry == ENTRY_ENRAGED_WATER_SPIRIT then
        st.stormSpell = SPELL_STORMBOLT
        st.stormTimer = math.random(0, 1000)
        st.stormReArm = function() return 17000 + math.random(0, 6000) end
    elseif entry == ENTRY_ENRAGED_FIRE_SPIRIT then
        -- The !GetAura(SPELL_FEL_FIRE_AURA) gated self-cast
        -- has no aura-state bridge — unmodeled (phase_hunter
        -- precedent; deviation in the header).
        st.stormSpell = SPELL_FEL_FIREBALL
        st.stormTimer = 2000 + math.random(0, 10000)
        st.stormReArm = function() return 6000 + math.random(0, 6000) end
    elseif entry == ENTRY_ENRAGED_EARTH_SPIRIT then
        -- Same aura-gate deviation as fire (see above).
        st.stormSpell = SPELL_FIERY_BOULDER
        st.stormTimer = 3000 + math.random(0, 4000)
        st.stormReArm = function() return 6000 + math.random(0, 3000) end
    elseif entry == ENTRY_ENRAGED_AIR_SPIRIT then
        st.chainLightningTimer = 10000
        st.hurricaneTimer = nil
    end
    spiritState[guid] = st
    spiritPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end

-- C++ Reset() (called on evade): empty in C++ — the
-- observable reset is the pump cancel + state drop, which
-- JustEngagedWith re-latches on re-engage.
local function onLeaveCombat(_, creature)
    fullReset(creature:GetGUID())
end

-- The C++ UpdateAI per-tick enrage latch (fire + air only)
-- lands on OnDamageTaken (nagrand_banner precedent): the
-- strict-fraction HealthBelowPctDamaged(35) check — health
-- after this hit strictly below 35% (shahraz precedent) —
-- with a used latch (the !GetAura re-cast arm has no
-- aura-state bridge — phase_hunter precedent).
for _, entry in ipairs({ ENTRY_ENRAGED_FIRE_SPIRIT, ENTRY_ENRAGED_AIR_SPIRIT }) do
    RegisterCreatureEvent(entry, 9, function(_, creature, attacker, damage)
        local guid = creature:GetGUID()
        local st = spiritState[guid]
        if not st or st.usedEnrage then
            return
        end
        local maxHealth = creature:GetMaxHealth()
        if maxHealth == 0 then
            return
        end
        if (creature:GetHealth() - damage) * 100 / maxHealth < 35 then
            st.usedEnrage = true
            creature:CastSpell(creature, SPELL_ENRAGE)
        end
    end)
end

for _, entry in ipairs({
    ENTRY_ENRAGED_EARTH_SPIRIT,
    ENTRY_ENRAGED_FIRE_SPIRIT,
    ENTRY_ENRAGED_AIR_SPIRIT,
    ENTRY_ENRAGED_WATER_SPIRIT,
}) do
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, onLeaveCombat)
    -- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
    -- this lands the same reset as the evade hook.
    RegisterCreatureEvent(entry, 23, onLeaveCombat)
end
