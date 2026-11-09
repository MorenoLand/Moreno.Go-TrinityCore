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
-- GUID-KEY FIX tick 511: timers + state keyed on GetGUIDLow()
-- (stable uint32) — GetGUID() userdata keys never match across
-- calls, so leave/died/reset cancels previously missed and the
-- ev21 split-personality death tracker never found its state
-- (tick 508 hadronox precedent).
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell) = DoCastSelf.
-- Ported arms (C++-exact at 1s granularity, documented below):
-- 2026-10-25: randomThreatTarget alive-filtered (C++ SelectTarget
-- skips offline refs, UnitAI.h); phase latches in floor form
-- (HealthBelowPct = health < CalculatePct trunc, Unit.h:872).
-- JustEngagedWith Talk(SAY_AGGRO 0) (event 1; BossAI-less
-- ScriptedAI, no instance leg in C++ JustEngagedWith) + 1s pump;
-- pump arms in C++ order: waiting branch (StopMoving +
-- AttackStop each tick, 4s, then RemoveFlag NOT_SELECTABLE);
-- Phase 1/3 merge branch (all three magi dead -> CenterOfRoom
-- leg skipped, see below; DoCastSelf TELESTRA_BACK 47714 +
-- SetVisible(true) + Phase 2/4 + waiting 4s + Talk SAY_MERGE 3;
-- C++ falls through in the merge tick to the split checks +
-- timer arms, ported 2026-11-05 — no early return after merge);
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
-- SetVisible(true); Reset / JustEngagedWith / JustDied
-- SetBossState(DATA_MAGUS_TELESTRA, NOT_STARTED/IN_PROGRESS/
-- DONE) PORTED 2026-11-05 — creature:SetBossState bridged
-- (lua_creature_events.go:5917; SetBossState census tick 249),
-- the old "no instance bridge" claim was stale.
-- Unmodeled (no bridges — documented, not wired):
-- Reset heroic Winter Veil hat aura (no game-event bridge);
-- split CastStop (creature:StopSpellCast IS bridged,
-- lua_creature_events.go:1022, but the leg is vacuous: all
-- three spells are instant, tick-225 verdict); split
-- UpdatePosition(CenterOfRoom)
-- (no position-set bridge — the boss reappears where it
-- split instead of room center); magi AttackStart relay
-- ported 2026-10-24 (summoned:EngageWithTarget on a random
-- threat target, C++ SplitPersonality order); magi spellcasting
-- (magi have
-- no AI class in the C++ file — their live casts are
-- DB-side SmartAI, not in this tree); DoAction
-- ACTION_MAGUS_DEAD + GetData DATA_SPLIT_PERSONALITY
-- — PORTED tick 417 via event 21 (telestraSummonedDied below): each
-- split magus (26928/26929/26930) dying records os.time() (the C++
-- GameTime::GetGameTime() leg, boss_magus_telestra.cpp:165-177); when
-- the third death of a split arrives with both gaps < 5s,
-- splitPersonality++ and the count is written to the boss motion's
-- SetData store (the merge leg clears the death times, :260-264).
-- Reset writes 0 (Initialize, :96-109).
-- achievement_split_personality — PORTED tick 417 via the SetData
-- write leg + Go gate (bossLatchData, achievement_criteria_scripts.go).

local ENTRY_TELESTRA = 26731

local DATA_MAGUS_TELESTRA = 1 -- nexus.h:31 (NEXDataTypes)
local DATA_SPLIT_PERSONALITY = 2 -- boss_magus_telestra.cpp:62
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

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

local GO_TELESTRAS_CONTAINMENT_SPHERE = 188526 -- nexus.h:65
local GAMEOBJECT_FLAGS = 9 -- arlokk precedent
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641
local GO_SEARCH_DIST = 1000 -- whole-instance radius; entries are unique per GO

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
    local alive = {}
    for _, t in ipairs(targets) do
        if t:IsAlive() then
            alive[#alive + 1] = t
        end
    end
    if #alive == 0 then
        return nil
    end
    return alive[math.random(#alive)]
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
            -- C++ falls through in the merge tick (no return after the
            -- merge leg): phase is now 2/4, so the split checks below
            -- and the timer arms run in this same tick.
        else
            schedule(guid, "pump", 1000, function()
                telestraPump(creature, guid)
            end)
            return
        end
    end
    if st.phase == 0 and creature:GetHealth() < math.floor(creature:GetMaxHealth() * 50 / 100) then
        splitPersonality(creature, st, 1)
        schedule(guid, "pump", 1000, function()
            telestraPump(creature, guid)
        end)
        return
    end
    if creature:IsHeroic() and st.phase == 2
            and creature:GetHealth() < math.floor(creature:GetMaxHealth() * 10 / 100) then
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
    local guid = creature:GetGUIDLow()
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
        deathTimes = {},
        splitPersonality = 0,
    }
    creature:Talk(SAY_AGGRO)
    creature:SetBossState(DATA_MAGUS_TELESTRA, BOSS_STATE_IN_PROGRESS) -- C++ JustEngagedWith: Talk, then instance->SetBossState(IN_PROGRESS)
    schedule(guid, "pump", 1000, function()
        telestraPump(creature, guid)
    end)
end

local function telestraLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function telestraTargetDied(event, creature, victim)
    if victim ~= nil and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function telestraDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    creature:SetVisible(true)
    creature:SetBossState(DATA_MAGUS_TELESTRA, BOSS_STATE_DONE) -- C++ JustDied: Talk, SetVisible, instance->SetBossState(DONE)
    -- C++ SetBossState override (instance_nexus.cpp:133-141): Telestra
    -- DONE frees her containment sphere (RemoveFlag NOT_SELECTABLE).
    -- No boss-state-change event exists, so the leg rides the DONE
    -- write here (palehoof precedent); the instanceData flag feeds
    -- the instance OnGameObjectCreate / OnPlayerEnter applies.
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data.telestraDone = true
    end
    local sphere = creature:FindNearestGameObject(GO_TELESTRAS_CONTAINMENT_SPHERE, GO_SEARCH_DIST)
    if sphere then
        sphere:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    end
    cancelTimers(creature:GetGUIDLow())
end

local function telestraReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    state[creature:GetGUIDLow()] = nil
    -- C++ Reset -> Initialize: splitPersonality = 0, time[] zeroed
    -- (boss_magus_telestra.cpp:96-109) -> GetData(DATA_SPLIT_PERSONALITY)
    -- = 0; achievement_split_personality read leg (tick 417).
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time zero would clobber the fight's count before
    -- the achievement gate reads it.
    if creature:IsAlive() then
        creature:SetData(DATA_SPLIT_PERSONALITY, 0)
    end
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetVisible(true)
    creature:SetBossState(DATA_MAGUS_TELESTRA, BOSS_STATE_NOT_STARTED) -- C++ Reset: instance->SetBossState(NOT_STARTED), unconditional (ScriptedAI direct call)
end

-- C++ DoAction(ACTION_MAGUS_DEAD) (boss_magus_telestra.cpp:165-177),
-- fed by SummonedCreatureDespawn (:219-240): each split magus death
-- records the time; when the third death of a split arrives with both
-- gaps < 5s, splitPersonality++. Event 21 (SummonedCreatureDied) is
-- the death-time twin — it fires at the kill, where the C++ despawn
-- time is death + the temp-summon corpse delay, and the 5s windows
-- tolerate that shift; a live despawn (e.g. post-merge) never fires
-- 21, matching the C++ IsAlive() guard. Death times clear at the
-- merge, mirroring the C++ time[] reset (:260-264).
local function telestraSummonedDied(event, creature, summon, killer)
    local entry = summon:GetEntry()
    if entry ~= NPC_FIRE_MAGUS and entry ~= NPC_FROST_MAGUS and entry ~= NPC_ARCANE_MAGUS then
        return
    end
    local st = state[creature:GetGUIDLow()]
    if st == nil then
        return
    end
    st.deathTimes[#st.deathTimes + 1] = os.time()
    if #st.deathTimes == 3 then
        local t = st.deathTimes
        if t[3] - t[2] < 5 and t[2] - t[1] < 5 then
            st.splitPersonality = st.splitPersonality + 1
            creature:SetData(DATA_SPLIT_PERSONALITY, st.splitPersonality)
        end
        st.deathTimes = {}
    end
end

RegisterCreatureEvent(ENTRY_TELESTRA, 1, telestraEnterCombat)
RegisterCreatureEvent(ENTRY_TELESTRA, 2, telestraLeaveCombat)
RegisterCreatureEvent(ENTRY_TELESTRA, 3, telestraTargetDied)
RegisterCreatureEvent(ENTRY_TELESTRA, 4, telestraDied)
RegisterCreatureEvent(ENTRY_TELESTRA, 21, telestraSummonedDied)
RegisterCreatureEvent(ENTRY_TELESTRA, 23, telestraReset)
