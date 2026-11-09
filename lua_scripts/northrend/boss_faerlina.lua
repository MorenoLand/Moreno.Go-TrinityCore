-- Grand Widow Faerlina (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_faerlina.cpp
-- (boss_faerlina (BossAI); npc_faerlina_add (ScriptedAI);
-- at_faerlina_entrance (OnlyOnceAreaTriggerScript);
-- achievement_momma_said_knock_you_out (AchievementCriteriaScript);
-- AddSC_boss_faerlina registers all four; loader decl 70 / call 265 per
-- northrend_script_loader.cpp — the TWELFTH Naxxramas group in
-- AddNorthrendScripts(), under the "// Naxxramas" marker; the call
-- before it is AddSC_boss_four_horsemen() (decl 69 / call 264), the
-- call after is AddSC_boss_heigan() (decl 71 / call 266)).
-- Entry: 15953 Faerlina (naxxramas.h NPC_FAERLINA :89 — kalecgos pass;
-- instance_naxxramas.cpp binds NPC_FAERLINA -> DATA_FAERLINA GUID
-- (case NPC_FAERLINA, line 136) and the boss index -> BOSS_FAERLINA
-- :31; the CreatureScript ScriptName binding is instance-shimmed, the
-- creature_template binding DB-side as usual).
-- Sole-source verified: whole-scripts-tree grep for "boss_faerlina" /
-- "npc_faerlina_add" / "at_faerlina_entrance" /
-- "achievement_momma_said_knock_you_out" hits boss_faerlina.cpp only
-- (loader carries only the decl/call lines); zero sql/ hits. No
-- faerlina lua existed. Talk() count = 6 grep-confirmed (zero spaced
-- "Talk (" variant) — ALL SIX accounted below.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 14 OnHitBySpell, 23 OnReset. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady (jedoga bar —
-- engagement-only leg, documented). The EVENT_POISON timer rides
-- CreateLuaEvent (the maiden convention), cancelled on 2/4/23.
-- Ported arms (C++-exact for all modeled arms):
-- boss_faerlina: JustEngagedWith Talk(SAY_AGGRO 1) (event 1; the
-- BossAI::JustEngagedWith base arms PORTED (SetBossState IN_PROGRESS +
-- pulse + me-form DoZoneInCombat, base-first) and the summons.
-- DoZoneInCombat leg PORTED 2026-11-05 (SummonAdds returns the adds
-- into faerlinaSummons, razuvious precedent; ev1 zones them in); the
-- EVENT_POISON / EVENT_FIRE / EVENT_FRENZY schedules seed the
-- CreateLuaEvent machine below); KilledUnit Talk(SAY_SLAY 2) (event 3 —
-- C++-GATED: the Talk fires only when victim->GetTypeId()==
-- TYPEID_PLAYER — Lua expresses it via victim:GetObjectType() ==
-- "Player" (nalorakk / kelthuzad player-gate precedent)); JustDied
-- Talk(SAY_DEATH 3) (event 4; _JustDied legs PORTED 2026-11-05 —
-- SetBossState DONE + summons.DespawnAll, moroes/razuvious precedent).
-- EVENT_FIRE — PORTED (re-sweep 2026-10-17: the "no random-target
-- SelectTarget bridge (cairne/kazzak)" claim is RETRACTED —
-- randomPlayer is the in-tree bridge, anomalus / eadric
-- precedent): SelectTarget(Random, 0) -> DoCast(RAIN_OF_FIRE
-- 28794) non-triggered; init randtime(6s,18s), Repeat
-- randtime(6s,18s) unconditional (outside the C++ target-null if);
-- no cast on an empty pool.
-- EVENT_POISON — PORTED (retraction re-audit 2026-10-14: the
-- "no DoCastAOE bridge / no aura-check bridge" claims are stale):
-- DoCastAOE(POISON_BOLT_VOLLEY 28796) non-triggered, gated on
-- !me->HasAura(SPELL_WIDOWS_EMBRACE_HELPER) — DoCastAOE lowers to
-- creature:CastSpell(creature, spell) (boss_ai.go DoCastAOE self-cast
-- convention — selin_fireheart / mr_smite / razuvious precedent) and
-- the aura gate bridges via creature:HasAura (the freya
-- DRAINED_OF_POWER precedent); SPELL_WIDOWS_EMBRACE_HELPER is
-- RAID_MODE(28732, 54097) — no raid-size (10/25-man) selector, so
-- both IDs are checked (the applied aura is always the
-- raid-size-appropriate one); init randtime(10s,15s) (:104), repeat
-- randtime(8s,15s).
-- EVENT_FRENZY — PORTED (2026-10-25: the "no aura-duration bridge" claim is
-- RETRACTED — creature:GetDuration is bridged, lua_creature_events.go:582,
-- tournament-dummy precedent): with GetAura(SPELL_WIDOWS_EMBRACE_HELPER)
-- active it skips the cast and reschedules at aura duration +1ms
-- (RAID_MODE 28732/54097 — both IDs checked, the applied aura is always
-- the raid-size-appropriate one); else DoCastSelf(FRENZY 28798)
-- non-triggered + Talk(EMOTE_FRENZY 5); init Minutes(1)+randtime(0s,20s)
-- (:106), repeat same.
-- CASTING gates PORTED 2026-11-04 (500ms IsCasting defer, mal_ganis
-- idiom): boss UpdateAI pre-loop :156 + in-loop :185 gates cover
-- EVENT_POISON / EVENT_FIRE / EVENT_FRENZY — defer precedes the
-- aura read (poison), target pick (fire), and aura-duration read
-- (frenzy); the event stays queued, no re-arm consumed. Talk /
-- SpellHit arms ungated per C++ (separate hooks). npc_faerlina_add
-- UpdateAI :239 pre-loop gate covers its unported fireball tick
-- only — add stays entry-blocked, no Lua bearer.
-- SpellHit — PORTED (re-sweep 2026-10-17: the "SpellHit never
-- fires in the Lua surface (SpellHit ruling)" claim is RETRACTED —
-- CREATURE_EVENT_ON_HIT_BY_SPELL (event 14) IS live and fires when a
-- creature is hit (boss_ai.go:2012-2033), args (event, creature,
-- caster, spellid)): spellId == RAID_MODE helper 28732/54097 ->
-- Talk(EMOTE_WIDOW_EMBRACE 4, caster). The Unit::Kill(me,
-- unitCaster) leg is provably inert (the caster is the
-- already-dead add casting from JustDied); the _frenzyDispels
-- counter has no bridged consumer (achievement-only) — both
-- documented with the handler.
-- SummonAdds — PORTED 2026-11-05 (creature:SummonCreatureGroup
-- bridged tick 238; creature:Is25ManRaid bridged): event 5
-- (InitializeAI leg, DONE-gated via creature:GetBossState,
-- razuvious precedent) + event 24 (JustReachedHome leg); the
-- returned adds land in faerlinaSummons, despawned on ev2/ev4/ev23
-- (the _Reset/_JustDied DespawnAll legs). Reset — _Reset legs
-- PORTED 2026-11-05 (SetBossState NOT_STARTED, != DONE-gated per
-- ScriptedCreature.cpp:509-510); _frenzyDispels=0 has no consumer
-- (achievement-only, below).
-- Unmodeled (no bridges — documented, not wired):
-- GetData(DATA_FRENZY_DISPELS 1) — consumed by the achievement (no
-- achievement bridge); at_faerlina_entrance Talk(SAY_GREET 0) via
-- instance GetGuidData(DATA_FAERLINA), NOT_STARTED-gated — no
-- area-trigger bridge (anubrekhan entrance precedent); achievement_
-- momma_said_knock_you_out (GetData(DATA_FRENZY_DISPELS)==0) — no
-- achievement bridge (tharon_ja precedent); npc_faerlina_add — PORTED
-- 2026-11-05 in lua_scripts/northrend/npc_faerlina_add.lua
-- (entries NPC_NAXXRAMAS_FOLLOWER 16505 / NPC_FOLLOWER_WORSHIPPER
-- 16506, naxxramas.h:105-106 — the old "entries live in DB summon
-- groups, unverifiable in the C++ tree" bar is retired; the entries
-- are the summon-group rows' targets, verified in the header):
-- Reset 10-man BIND/CHARM immunities (ApplySpellImmune + Is25ManRaid
-- bridges), JustEngagedWith cross-AI DoZoneInCombat via
-- GetInstanceCreature(DATA_FAERLINA) (ev5 SetGuidData latch on the
-- boss, razuvious/understudy precedent), JustDied 10-man
-- DoCast(WIDOWS_EMBRACE 28732) on Faerlina (ev4 cast from a dead
-- creature, razuvious hopeless precedent), UpdateAI DoCastVictim(
-- ADD_FIREBALL 54095) as an ev7 pump (understudy precedent).
-- FIDELITY NOTE: the C++ hard-codes 54095 on both difficulties — the
-- "// 25-man: 54096" enum comment is stale vs the C++ text; a future
-- port must NOT raid-mode it.

local ENTRY_FAERLINA = 15953

local BOSS_FAERLINA = 1 -- naxxramas.h NAXEncounter (ANUBREKHAN 0, FAERLINA 1)
local DATA_FAERLINA = 1 -- naxxramas.h NAXData64 (ANUBREKHAN 0, FAERLINA 1)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local SUMMON_GROUP_WORSHIPPERS = 1
local SUMMON_GROUP_FOLLOWERS = 2

local SAY_AGGRO = 1
local SAY_SLAY = 2
local SAY_DEATH = 3

local SPELL_POISON_BOLT_VOLLEY = 28796
local SPELL_RAIN_OF_FIRE = 28794
local SPELL_WIDOWS_EMBRACE_10 = 28732
local SPELL_WIDOWS_EMBRACE_25 = 54097
local SPELL_FRENZY = 28798
local EMOTE_WIDOW_EMBRACE = 4
local EMOTE_FRENZY = 5

-- CreateLuaEvent timer harness (GUID-keyed timers table, cancel on
-- events 2/4/23) — the maiden / bjarngrim-lieutenant convention.
local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
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

-- C++ SummonAdds (boss_faerlina.cpp:77-82): SummonCreatureGroup(
-- SUMMON_GROUP_WORSHIPPERS 1) + 25-man SUMMON_GROUP_FOLLOWERS 2.
-- The returned adds land in faerlinaSummons (the C++ `summons`
-- container twin, razuvious precedent); ev2/ev4/ev23 despawn them
-- (the _Reset/_JustDied summons.DespawnAll legs,
-- ScriptedCreature.cpp:507/516). A group with no DB rows returns an
-- empty table (the bridge's C++ `if (!data) return;` twin), so the
-- port is inert until the live TDB provides creature_summon_groups
-- rows — same caveat as the tick-238 group ports.
local faerlinaSummons = {}

local function summonAdds(creature, guid)
    local adds = faerlinaSummons[guid]
    if not adds then
        adds = {}
        faerlinaSummons[guid] = adds
    end
    for _, add in ipairs(creature:SummonCreatureGroup(SUMMON_GROUP_WORSHIPPERS)) do
        adds[#adds + 1] = add
    end
    if creature:Is25ManRaid() then
        for _, add in ipairs(creature:SummonCreatureGroup(SUMMON_GROUP_FOLLOWERS)) do
            adds[#adds + 1] = add
        end
    end
end

local function despawnAdds(guid)
    local adds = faerlinaSummons[guid]
    if adds then
        for _, add in ipairs(adds) do
            if add ~= nil then
                add:Despawn()
            end
        end
        faerlinaSummons[guid] = nil
    end
end

-- C++ SelectTarget(SelectTargetMethod::Random, 0) — no distance
-- filter, no playerOnly flag: it draws the THREAT LIST (UnitAI.h:
-- "from the threat list", SelectTargetList skips offline refs at
-- build), not every player in the instance. The old GetPlayersInWorld
-- pool could pick players never engaged with the boss — fixed
-- 2026-10-25, same stale-pool class as the eadric / baltharus /
-- archavon pools. Any-unit (playerOnly defaults false), alive
-- (dead refs read offline, ThreatManager ShouldBeOffline).
local function randomPlayer(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ UpdateAI EVENT_POISON (boss_faerlina.cpp:162-166):
-- DoCastAOE(SPELL_POISON_BOLT_VOLLEY 28796) non-triggered, gated on
-- !me->HasAura(SPELL_WIDOWS_EMBRACE_HELPER), randtime(10s,15s) init
-- (:104) / randtime(8s,15s) repeat.
local function poisonTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "poison", 500, function()
            poisonTick(creature, guid)
        end)
        return
    end
    if not creature:HasAura(SPELL_WIDOWS_EMBRACE_10)
        and not creature:HasAura(SPELL_WIDOWS_EMBRACE_25) then
        creature:CastSpell(creature, SPELL_POISON_BOLT_VOLLEY)
    end
    schedule(guid, "poison", {8000, 15000}, function()
        poisonTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_FIRE (boss_faerlina.cpp:167-171):
-- SelectTarget(Random, 0) -> DoCast(RAIN_OF_FIRE 28794),
-- non-triggered; the Repeat(randtime(6s,18s)) is UNCONDITIONAL
-- (outside the target-null if); init randtime(6s,18s). No cast on
-- an empty pool (C++ SelectTarget null leg).
local function fireTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fire", 500, function()
            fireTick(creature, guid)
        end)
        return
    end
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_RAIN_OF_FIRE)
    end
    schedule(guid, "fire", {6000, 18000}, function()
        fireTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_FRENZY (boss_faerlina.cpp:173-182):
-- with GetAura(SPELL_WIDOWS_EMBRACE_HELPER) active, skip the cast
-- and reschedule at aura duration +1ms; else DoCastSelf(FRENZY
-- 28798) non-triggered + Talk(EMOTE_FRENZY 5), repeat
-- Minutes(1)+randtime(0s,20s). Init Minutes(1)+randtime(0s,20s)
-- (:106). RAID_MODE helper: both IDs checked like the poison arm.
local function frenzyTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frenzy", 500, function()
            frenzyTick(creature, guid)
        end)
        return
    end
    if creature:HasAura(SPELL_WIDOWS_EMBRACE_10) then
        schedule(guid, "frenzy", creature:GetDuration(SPELL_WIDOWS_EMBRACE_10) + 1, function()
            frenzyTick(creature, guid)
        end)
    elseif creature:HasAura(SPELL_WIDOWS_EMBRACE_25) then
        schedule(guid, "frenzy", creature:GetDuration(SPELL_WIDOWS_EMBRACE_25) + 1, function()
            frenzyTick(creature, guid)
        end)
    else
        creature:CastSpell(creature, SPELL_FRENZY)
        creature:Talk(EMOTE_FRENZY)
        schedule(guid, "frenzy", {60000, 80000}, function()
            frenzyTick(creature, guid)
        end)
    end
end

-- C++ InitializeAI (boss_faerlina.cpp:84-91): if not dead and boss
-- state != DONE, Reset() + SummonAdds(). The SetGuidData latch is the
-- C++ instance OnCreatureCreate bind (NPC_FAERLINA -> DATA_FAERLINA,
-- instance_naxxramas.cpp:136) in its Lua form (selin/crystal
-- precedent); the add's JustEngagedWith/JustDied read it back via
-- GetInstanceCreature(DATA_FAERLINA).
local function faerlinaSpawn(event, creature)
    creature:SetGuidData(DATA_FAERLINA)
    if not creature:IsDead() and creature:GetBossState(BOSS_FAERLINA) ~= BOSS_STATE_DONE then
        creature:SetBossState(BOSS_FAERLINA, BOSS_STATE_NOT_STARTED) -- Reset leg (:109-113)
        summonAdds(creature, creature:GetGUIDLow())
    end
end

-- C++ JustReachedHome (boss_faerlina.cpp:93-97): _JustReachedHome
-- (setActive(false), no Lua model) + SummonAdds().
local function faerlinaReachHome(event, creature)
    summonAdds(creature, creature:GetGUIDLow())
end

local function faerlinaEnterCombat(event, creature, target)
    creature:SetBossState(BOSS_FAERLINA, BOSS_STATE_IN_PROGRESS) -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:538)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    for _, add in ipairs(faerlinaSummons[guid] or {}) do -- C++ summons.DoZoneInCombat (:103)
        if add ~= nil then
            add:DoZoneInCombat()
        end
    end
    cancelTimers(guid)
    schedule(guid, "poison", {10000, 15000}, function()
        poisonTick(creature, guid)
    end)
    schedule(guid, "fire", {6000, 18000}, function()
        fireTick(creature, guid)
    end)
    schedule(guid, "frenzy", {60000, 80000}, function()
        frenzyTick(creature, guid)
    end)
end

-- C++ SpellHit (boss_faerlina.cpp:119-130): fires on the add's
-- deathrattle Widow's Embrace (the RAID_MODE helper 28732/54097 —
-- the 10-man add casts only 28732 on death, so the 25-man half of
-- the gate never fires there). Ported: Talk(EMOTE_WIDOW_EMBRACE
-- 4, caster). Dropped as provably inert: Unit::Kill(me,
-- unitCaster) — the caster is the already-dead add (it cast from
-- JustDied); C++ Unit::Kill on a dead unit is a no-op. Dropped
-- with no bridged consumer: the _frenzyDispels counter is PORTED
-- 2026-11-08 (tick 416) in faerlinaSpellHit below.
-- Firing note: the event-14 bridge fires once per hit target — the true
-- C++ DoDamageAndTriggers twin — so this is 1:1 with the C++ per-hit
-- SpellHit regardless of effect count (the earlier per-effect caveat is
-- retracted).
local DATA_FRENZY_DISPELS = 1 -- boss_faerlina.cpp:65

local function faerlinaSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_WIDOWS_EMBRACE_10 or spellId == SPELL_WIDOWS_EMBRACE_25 then
        creature:Talk(EMOTE_WIDOW_EMBRACE, caster)
        -- C++ SpellHit :128-140: ++_frenzyDispels (the
        -- achievement_momma_said_knock_you_out latch; Go case in
        -- engine/world/achievement_criteria_scripts.go). The
        -- Unit::Kill(me, unitCaster) leg stays dropped: the caster is
        -- the already-dead add, a no-op in C++.
        creature:SetData(DATA_FRENZY_DISPELS, creature:GetData(DATA_FRENZY_DISPELS) + 1)
    end
end

local function faerlinaLeaveCombat(event, creature)
    -- C++ EnterEvadeMode -> Reset -> _Reset summons.DespawnAll; the
    -- re-summon rides event 24 below. Death never fires event 2, so
    -- this stays evade-only, C++-exact (razuvious precedent).
    despawnAdds(creature:GetGUIDLow())
    cancelTimers(creature:GetGUIDLow())
end

local function faerlinaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function faerlinaDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    -- C++ JustDied (:121-125): _JustDied summons.DespawnAll +
    -- SetBossState DONE (ScriptedCreature.cpp:513-520; moroes
    -- precedent).
    despawnAdds(creature:GetGUIDLow())
    creature:SetBossState(BOSS_FAERLINA, BOSS_STATE_DONE)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset (boss_faerlina.cpp:109-113): _Reset (summons.DespawnAll +
-- SetBossState NOT_STARTED when state != DONE,
-- ScriptedCreature.cpp:499-511) + _frenzyDispels = 0 (achievement-only,
-- no consumer — documented in the header).
local function faerlinaReset(event, creature)
    despawnAdds(creature:GetGUIDLow())
    if creature:GetBossState(BOSS_FAERLINA) ~= BOSS_STATE_DONE then
        creature:SetBossState(BOSS_FAERLINA, BOSS_STATE_NOT_STARTED)
    end
    cancelTimers(creature:GetGUIDLow())
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time zero would clobber the fight's counter before
    -- the achievement gate reads it; the wipe re-arm is preserved.
    if creature:IsAlive() then
        creature:SetData(DATA_FRENZY_DISPELS, 0) -- C++ Reset :110-113
    end
end

RegisterCreatureEvent(ENTRY_FAERLINA, 1, faerlinaEnterCombat)
RegisterCreatureEvent(ENTRY_FAERLINA, 2, faerlinaLeaveCombat)
RegisterCreatureEvent(ENTRY_FAERLINA, 3, faerlinaTargetDied)
RegisterCreatureEvent(ENTRY_FAERLINA, 4, faerlinaDied)
RegisterCreatureEvent(ENTRY_FAERLINA, 5, faerlinaSpawn)
RegisterCreatureEvent(ENTRY_FAERLINA, 14, faerlinaSpellHit)
RegisterCreatureEvent(ENTRY_FAERLINA, 23, faerlinaReset)
RegisterCreatureEvent(ENTRY_FAERLINA, 24, faerlinaReachHome)
