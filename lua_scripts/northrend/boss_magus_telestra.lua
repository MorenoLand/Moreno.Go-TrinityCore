-- Grand Magus Telestra (The Nexus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Nexus/boss_magus_telestra.cpp
-- (boss_magus_telestra (ScriptedAI); AddSC_boss_magus_telestra also
-- registers achievement_split_personality, unmodeled below).
-- Entry: 26731 Grand Magus Telestra (checkpoint kg-49, wowhead-only;
-- no NPC_ const in the C++ tree and no creature_template row in
-- sql/sqlite/world.sql, same entry-block class as ormorok).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via a 1s pump (the
-- C++ UpdateAI control flow is literal here: shared uiCooldown
-- freezes all timers, split/waiting branches return early, so
-- independent per-arm timers would NOT reproduce it); melee is
-- engine-driven in Go, like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell) = DoCastSelf.
-- Ported arms (C++-exact at 1s granularity, documented below):
-- JustEngagedWith Talk(SAY_AGGRO 0) (event 1; BossAI-less
-- ScriptedAI, no instance leg in C++ JustEngagedWith) + 1s pump;
-- pump arms in C++ order: waiting branch (StopMoving +
-- AttackStop each tick, 4s, then RemoveFlag NOT_SELECTABLE);
-- Phase 1/3 merge branch (all three magi dead -> CenterOfRoom
-- leg skipped, see below; DoCastSelf TELESTRA_BACK 47714 +
-- SetVisible(true) + Phase 2/4 + waiting 4s + Talk SAY_MERGE 3);
-- Phase 0 HealthBelowPct(50) split + heroic Phase 2
-- HealthBelowPct(10) split (CastStop leg skipped, see below;
-- RemoveAllAuras + SetVisible(false) + SetFlag NOT_SELECTABLE
-- + summon fire/frost/arcane magi + Talk SAY_SPLIT 4);
-- shared uiCooldown (ice nova 1500ms / gravity well 6s /
-- firebomb 2s — 1500ms rounds to 2 pump ticks); ICE_NOVA
-- 7s init / 15s repeat (random threat target, non-triggered
-- 47772 / heroic 56935 — heroic spell id NOT switched: C++
-- uses SPELL_ICE_NOVA unconditionally, the H_ enum is unused);
-- GRAVITY_WELL 15s (victim, non-triggered 47756);
-- FIREBOMB 0s init / 2s repeat (random threat target,
-- non-triggered 47773 / heroic 56934 — same unused-H_ note).
-- Magi-death detection: C++ SummonedCreatureDespawn fires on
-- magus death, but Go event 20 fires only on timed-summon
-- expiry (pet_combat.go expireTempSummonGuardian), never on
-- death — so the pump polls FindNearestCreature for each
-- magus entry within 100yd instead (bridge filters Health==0,
-- lua_creature_events.go: FindNearestCreature). A magus that
-- never materializes (nil summon) counts as dead: C++ would
-- strand the boss invisible forever on a failed summon, the
-- poll merges instead (deliberate anti-strand deviation).
-- KilledUnit Talk(SAY_KILL 1) player-gated (event 3,
-- commanders precedent); JustDied Talk(SAY_DEATH 2) +
-- SetVisible(true).
-- Unmodeled (no bridges — documented, not wired): Reset /
-- JustEngagedWith / JustDied instance bookkeeping
-- (SetBossState DATA_MAGUS_TELESTRA, no instance bridge);
-- Reset heroic Winter Veil hat aura (no game-event bridge);
-- split CastStop (InterruptNonMeleeSpells is a no-op twin,
-- not a cast stop); split UpdatePosition(CenterOfRoom)
-- (no position-set bridge — the boss reappears where it
-- split instead of room center); magi AttackStart relay
-- ported 2026-10-24 (summoned:EngageWithTarget on a random
-- threat target, C++ SplitPersonality order); magi spellcasting
-- (magi have
-- no AI class in the C++ file — their live casts are
-- DB-side SmartAI, not in this tree); DoAction
-- ACTION_MAGUS_DEAD + GetData DATA_SPLIT_PERSONALITY
-- (consumed only by achievement_split_personality, no
-- achievement bridge); the 5s split-personality timing
-- window dies with that achievement leg.

local ENTRY_TELESTRA = 26731

local NPC_FIRE_MAGUS = 26928
local NPC_FROST_MAGUS = 26930
local NPC_ARCANE_MAGUS = 26929

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_DEATH = 2
local SAY_MERGE = 3
local SAY_SPLIT = 4

local SPELL_ICE_NOVA = 47772
local SPELL_FIREBOMB = 47773
local SPELL_GRAVITY_WELL = 47756
local SPELL_TELESTRA_BACK = 47714
local SPELL_FIRE_MAGUS_VISUAL = 47705
local SPELL_FROST_MAGUS_VISUAL = 47706
local SPELL_ARCANE_MAGUS_VISUAL = 47704

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local timers = {}
local state = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    state[guid] = nil
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    if targets == nil or #targets == 0 then
        return nil
    end
    return targets[math.random(#targets)]
end

local function splitPersonality(creature, st, newPhase)
    st.phase = newPhase
    creature:RemoveAllAuras()
    creature:SetVisible(false)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    local magi = {
        { NPC_FIRE_MAGUS, SPELL_FIRE_MAGUS_VISUAL },
        { NPC_FROST_MAGUS, SPELL_FROST_MAGUS_VISUAL },
        { NPC_ARCANE_MAGUS, SPELL_ARCANE_MAGUS_VISUAL },
    }
    for _, magus in ipairs(magi) do
        local summoned = creature:SummonCreature(magus[1],
            creature:GetX(), creature:GetY(), creature:GetZ(),
            creature:GetO(), 3, 1000)
        if summoned ~= nil then
            summoned:CastSpell(summoned, magus[2])
            local target = randomThreatTarget(creature)
            if target ~= nil then
                summoned:EngageWithTarget(target)
            end
        end
    end
    st.fireDead = false
    st.frostDead = false
    st.arcaneDead = false
    creature:Talk(SAY_SPLIT)
end

local function telestraPump(creature, guid)
    local st = state[guid]
    if st == nil then
        return
    end
    if st.waiting then
        creature:StopMoving()
        creature:AttackStop()
        st.waitTimer = st.waitTimer - 1000
        if st.waitTimer <= 0 then
            creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            st.waiting = false
        end
        schedule(guid, "pump", 1000, function()
            telestraPump(creature, guid)
        end)
        return
    end
    if st.phase == 1 or st.phase == 3 then
        if creature:FindNearestCreature(NPC_FIRE_MAGUS, 100) == nil then
            st.fireDead = true
        end
        if creature:FindNearestCreature(NPC_FROST_MAGUS, 100) == nil then
            st.frostDead = true
        end
        if creature:FindNearestCreature(NPC_ARCANE_MAGUS, 100) == nil then
            st.arcaneDead = true
        end
        if st.fireDead and st.frostDead and st.arcaneDead then
            creature:CastSpell(creature, SPELL_TELESTRA_BACK)
            creature:SetVisible(true)
            if st.phase == 1 then
                st.phase = 2
            else
                st.phase = 4
            end
            st.waiting = true
            st.waitTimer = 4000
            creature:Talk(SAY_MERGE)
        end
        schedule(guid, "pump", 1000, function()
            telestraPump(creature, guid)
        end)
        return
    end
    if st.phase == 0 and creature:GetHealthPct() < 50 then
        splitPersonality(creature, st, 1)
        schedule(guid, "pump", 1000, function()
            telestraPump(creature, guid)
        end)
        return
    end
    if creature:IsHeroic() and st.phase == 2
            and creature:GetHealthPct() < 10 then
        splitPersonality(creature, st, 3)
        schedule(guid, "pump", 1000, function()
            telestraPump(creature, guid)
        end)
        return
    end
    if st.cooldown > 0 then
        if st.cooldown <= 1000 then
            st.cooldown = 0
        else
            st.cooldown = st.cooldown - 1000
            schedule(guid, "pump", 1000, function()
                telestraPump(creature, guid)
            end)
            return
        end
    end
    st.iceNova = st.iceNova - 1000
    if st.iceNova <= 0 then
        local target = randomThreatTarget(creature)
        if target ~= nil then
            creature:CastSpell(target, SPELL_ICE_NOVA)
            st.cooldown = 1500
        end
        st.iceNova = 15000
    end
    st.gravityWell = st.gravityWell - 1000
    if st.gravityWell <= 0 then
        local victim = creature:GetVictim()
        if victim ~= nil then
            creature:CastSpell(victim, SPELL_GRAVITY_WELL)
            st.cooldown = 6000
        end
        st.gravityWell = 15000
    end
    st.fireBomb = st.fireBomb - 1000
    if st.fireBomb <= 0 then
        local target = randomThreatTarget(creature)
        if target ~= nil then
            creature:CastSpell(target, SPELL_FIREBOMB)
            st.cooldown = 2000
        end
        st.fireBomb = 2000
    end
    schedule(guid, "pump", 1000, function()
        telestraPump(creature, guid)
    end)
end

local function telestraEnterCombat(event, creature)
    local guid = creature:GetGUID()
    state[guid] = {
        phase = 0,
        iceNova = 7000,
        fireBomb = 0,
        gravityWell = 15000,
        cooldown = 0,
        waiting = false,
        waitTimer = 0,
        fireDead = false,
        frostDead = false,
        arcaneDead = false,
    }
    creature:Talk(SAY_AGGRO)
    schedule(guid, "pump", 1000, function()
        telestraPump(creature, guid)
    end)
end

local function telestraLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function telestraTargetDied(event, creature, victim)
    if victim ~= nil and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function telestraDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    creature:SetVisible(true)
    cancelTimers(creature:GetGUID())
end

local function telestraReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetVisible(true)
end

RegisterCreatureEvent(ENTRY_TELESTRA, 1, telestraEnterCombat)
RegisterCreatureEvent(ENTRY_TELESTRA, 2, telestraLeaveCombat)
RegisterCreatureEvent(ENTRY_TELESTRA, 3, telestraTargetDied)
RegisterCreatureEvent(ENTRY_TELESTRA, 4, telestraDied)
RegisterCreatureEvent(ENTRY_TELESTRA, 23, telestraReset)
