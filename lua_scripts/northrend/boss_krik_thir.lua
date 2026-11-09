-- Krik'thir the Gatewatcher (Azjol-Nerub dungeon) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/AzjolNerub/boss_krikthir_the_gatewatcher.cpp
-- (boss_krik_thir). Azjol-Nerub dungeon-script unit per
-- northrend_script_loader.cpp order (decl 27 / call 229, immediately
-- after AddSC_instance_ahnkahet(); the Azjol-Nerub dungeon block
-- opens here).
-- Entry: 28684 Krik'thir the Gatewatcher (azjol_nerub.h NPC_KRIKTHIR —
-- kalecgos pass; the GetAzjolNerubAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim (moroes precedent).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler (gargolmar
-- precedent); BossAI _Reset/_JustDied instance bookkeeping has no bridge.
-- Ported arms (C++-exact for all modeled arms):
-- boss_krik_thir: EVENT_MIND_FLAY DoCastVictim(Mind Flay 52586),
-- randtime(1s,3s) init (C++ JustEngagedWith schedule; the passive
-- engage legs are unbridged but the arm itself is port-pattern-ready),
-- 9-11s repeat (math.random — slad_ran precedent); KilledUnit
-- Talk(SAY_SLAY 1) player-gated (event 3,
-- victim:GetObjectType()=="Player" — nalorakk precedent); JustDied
-- Talk(SAY_DEATH 2) (event 4; the summons.clear() + _JustDied()
-- instance-bookkeeping legs have no bridge).
-- EVENT_SWARM — DoCastAOE(SPELL_SWARM 52440) self-cast +
-- Talk(SAY_SWARM 3), 5s init, no repeat (C++ does not reschedule;
-- one-shot timer).
-- REMOVED 2026-10-25: EVENT_ANUBAR_CHARGE / EVENT_BACKSTAB do NOT
-- belong to the boss — they are npc_anub_ar_skirmisher's arms
-- (boss_krikthir_the_gatewatcher.cpp:717-718, 735-743: charge 6-8s
-- init, 20-25s repeat, SelectTarget(Random, 0, 100, true) threat-list
-- player pick; backstab 7-9s init, 10-13s repeat, victim->isInBack(me)
-- gate). The 2026-10-23 "previously missed" port put the skirmisher's
-- rotation on entry 28684, so the boss wrongly charged/backstabbed;
-- both arms are removed here and live only in the skirmisher paragraph
-- below (entry-unverifiable, unregistered).
-- Unmodeled (no bridges — documented, not wired):
-- The passive pre-fight model — Reset leg SetReactState(REACT_PASSIVE)
-- (SetReactState IS bridged — frozen_core precedent,
-- lua_creature_events.go:524); JustAppeared -> SummonAdds()
-- (instance->GetBossState(DATA_KRIKTHIR)==DONE gate +
-- SummonCreatureGroup(1..3) with DATA_PET_GROUP stamping — the summon
-- STRAND is absent, standing) — so nothing below is emulated.
-- MoveInLineOfSight passive-engage leg (JustEngagedWith(who) when
-- passive and within attack distance,
-- boss_krikthir_the_gatewatcher.cpp:192-202) is fused to the
-- Reset-passive leg: Go's ev27 fires only at the aggro-engage branch
-- (lua_creature_events.go:1447), unreachable for a REACT_PASSIVE
-- creature — a one-sided Reset port would freeze the boss passive.
-- EVENT_SEND_GROUP — PORTED 2026-11-07 (tick 390): DoCastAOE
-- (SUBBOSS_AGGRO_TRIGGER 52343, triggered), 70s repeat, driven by the
-- DoAction machine below (PET_ENGAGED schedules it; a watcher death
-- reschedules it 5s out). The trigger's SpellScript watcher-target
-- selection stays out (no SpellScript-binding bridge, razelikh
-- precedent); the cast itself is C++-exact and inert without it.
-- EVENT_SWARM (DoCastAOE(SPELL_SWARM 52440) + Talk(SAY_SWARM 3), 5s
-- EVENT_FRENZY (HealthBelowPct(10) _hadFrenzy latch -> DoCastSelf(Frenzy
-- 28747) + DoCastAOE(CURSE_OF_FATIGUE 52592), 1s init, 15s repeat) —
-- PORTED (2026-10-21): the latch lives in postDamage math inside
-- krikThirDamageTaken (event 9). The C++ arm is UpdateAI:258
-- HealthBelowPct(10), not DamageTaken HealthBelowPctDamaged, so the
-- nefarian postDamage>0 guard keeps it from firing on the killing blow.
-- DoCastAOE leg rendered as self-cast per convention.
-- Cast-state PORTED: own UpdateAI pre-loop (:255) + in-loop (:290)
-- CASTING gates — mindflay, swarm, frenzy defer 500ms while
-- casting, no repeat re-arm (mal_ganis idiom). The _hadFrenzy
-- latch-set via event 9 stays ungated (DamageTaken-modeled leg,
-- taldaram precedent); the EVENT_FRENZY cast itself defers.
-- The DoAction machine — PORTED 2026-11-07 (tick 390)
-- (boss_krikthir_the_gatewatcher.cpp:214-251): ev38 krikThirDoAction
-- below. The tick-389 EnterEvadeMode bridge unblocked the consumer
-- (GetInstanceCreature :6157 + DoAction dispatch :827 were already
-- live). ACTION_GATEWATCHER_GREET (-1, instance SetData + Talk
-- SAY_PREFIGHT 4) stays out: instance-script model absent (standing).
-- SpellHit / SpellHitTarget (SUBBOSS_AGGRO_TRIGGER -> DoZoneInCombat
-- / Talk SAY_SEND_GROUP 5) stay out: the trigger cast has no modeled
-- targets in Go, so event 15 never fires for it (event 15 itself is
-- live — boss_ai.go:1904).
-- npc_watcher_gashra / npc_watcher_narjil / npc_watcher_silthik —
-- PORTED 2026-11-07 (tick 390): entries 28730 / 28729 / 28731
-- (azjol_nerub.h). Engage relays (petAI JustEngagedWith :330-352:
-- WATCHER_ENGAGED once via summoner + PET_ENGAGED once), JustDied
-- relays (:434/:512/:590 -> GetInstanceCreature(DATA_KRIKTHIR)
-- -> DoAction), and rotation arms (gashra ENRAGE/WEB_WRAP/
-- INFECTED_BITE; narjil BLINDING_WEBS/WEB_WRAP/INFECTED_BITE;
-- silthik POISON_SPRAY/WEB_WRAP/INFECTED_BITE — inits/repeats
-- C++-exact, see the rotation tables). WEB_WRAP target pick is the
-- in-tree randomTarget helper (GetAITargets + alive; player-only for
-- narjil/silthik per their SelectTarget(..., true), any-target for
-- gashra). DORMANT until the SummonCreatureGroup strand lands
-- (watchers spawn only from DB-side group summons) — phoenix-egg /
-- void-sentry precedent. The passive pre-fight model stays out
-- (REACT_PASSIVE stamping, group aggro, MoveInLineOfSight engage
-- leg — see the header note above); the engage relays fire from ev1
-- instead.
-- npc_anub_ar_warrior / npc_anub_ar_skirmisher /
-- npc_anub_ar_shadowcaster / npc_skittering_swarmer /
-- npc_skittering_infector / npc_gatewatcher_web_wrap — ENTRY
-- UNVERIFIABLE (no NPC constants in azjol_nerub.h or anywhere in the C++
-- sources; the adds are summoned from DB-side SummonCreatureGroup data,
-- not C++ evidence — belnistrasz/willix precedent, no invented
-- identifiers): zero registration; they join the bridgeable-but-entry-
-- blocked queue. Port-pattern-ready arms documented here: warrior
-- EVENT_CLEAVE DoCastVictim(49806) 7-9s init, 10-16s repeat +
-- EVENT_STRIKE DoCastVictim(52532) 5-10s init, 15-19s repeat; skirmisher
-- EVENT_ANUBAR_CHARGE 6-8s init, 20-25s repeat, threat-list player
-- pick (SelectTarget Random, 0, 100, true — NOT instance-wide) +
-- SpellHitTarget(CHARGE 52538) -> DoCast(unitTarget, FIXATE_TRIGGER
-- 52536) behind event-15 + EVENT_BACKSTAB 7-9s init, 10-13s repeat,
-- DoCastVictim(52540) gated on victim->isInBack(me)
-- (creature:IsInBack now bridged on the event surface — skirmisher stays
-- out on entry-blocked grounds, not bridge grounds); shadowcaster EVENT_SHADOW_NOVA DoCastVictim
-- (52535) 10-14s init, 10-16s repeat (EVENT_SHADOW_BOLT 52534 behind the
-- random-target SelectTarget bridge); skittering_swarmer InitializeAI
-- (instance->GetCreature(DATA_KRIKTHIR)->getAttackerForHelper()
-- AttackStart + gatewatcher->AI()->JustSummoned — instance model
-- absent); skittering_infector JustDied DoCastAOE(ACID_SPLASH 52446) —
-- stays out: host npc entry-unverifiable (summon STRAND absent) and
-- its InitializeAI is likewise instance-bound;
-- npc_gatewatcher_web_wrap (NullCreatureAI) JustDied -> summoner
-- RemoveAurasDueToSpell(WEB_WRAP_WRAPPED 52087) — the aura-removal
-- bridge EXISTS (creature:RemoveAura — the gal_darah / drakkari_colossus /
-- festergut precedent; the aeranas "absent" verdict is retracted
-- 2026-10-17) but the arm stays out: the host npc is
-- entry-unverifiable + summon STRAND absent (GetSummonerUnit IS
-- bridged, lua_creature_events.go:4999 — the "no summoner-unit
-- bridge" verdict is retracted 2026-11-07; the stay-out stands on
-- entry + summon-STRAND grounds alone).
-- spell_gatewatcher_subboss_trigger (52343 watcher target selection)
-- / spell_anub_ar_skirmisher_fixate (52536 -> 52537) /
-- spell_gatewatcher_web_wrap (52086 expire -> 52087) — no
-- SpellScript/AuraScript binding bridge on the Lua surface (razelikh
-- precedent) — documented-only.
-- achievement_watch_him_die — AchievementCriteriaScript (instance
-- GetCreature per watcher DATA_ entry, all-dead check) — no
-- achievement-criteria bridge (snakes precedent) + instance-script
-- model absent — documented-only.

local ENTRY_KRIK_THIR = 28684

local SAY_AGGRO = 0
local SAY_SLAY  = 1
local SAY_DEATH = 2
local SAY_SWARM = 3
local SAY_SEND_GROUP = 5

local SPELL_MIND_FLAY = 52586
local SPELL_SWARM = 52440
local SPELL_FRENZY = 28747
local SPELL_CURSE_OF_FATIGUE = 52592

local SPELL_SUBBOSS_AGGRO_TRIGGER = 52343
local SPELL_ENRAGE = 52470
local SPELL_BLINDING_WEBS = 52524
local SPELL_POISON_SPRAY = 52493
local SPELL_WEB_WRAP = 52086
local SPELL_INFECTED_BITE = 52469

local DATA_KRIKTHIR = 0 -- azjol_nerub.h:31
local ENTRY_WATCHER_NARJIL = 28729 -- azjol_nerub.h:50
local ENTRY_WATCHER_GASHRA = 28730 -- azjol_nerub.h:51
local ENTRY_WATCHER_SILTHIK = 28731 -- azjol_nerub.h:52

local ACTION_GASHRA_DIED = 0
local ACTION_NARJIL_DIED = 1
local ACTION_SILTHIK_DIED = 2
local ACTION_WATCHER_ENGAGED = 3
local ACTION_PET_ENGAGED = 4
local ACTION_PET_EVADE = 5

-- GUID-key rule: per-GUID tables are keyed on GetGUIDLow() (stable
-- number). GetGUID() returns a fresh UInt64 userdata per call and
-- Lua table keys compare userdata by identity, so a key stored
-- under one call never matches a lookup from another
-- (engine/scripting/object.go pushUInt64; instance_gundrak.lua
-- GUID-KEY FIX).
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

-- C++ EVENT_MIND_FLAY: DoCastVictim; randtime(1s,3s) init, 9-11s repeat.
local function mindFlayTick(creature, guid)
    -- Own UpdateAI pre-loop (:255) + in-loop (:290) CASTING gates:
    -- while casting the event stays queued — 500ms defer, no
    -- repeat re-arm (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "mindflay", 500, function()
            mindFlayTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_MIND_FLAY)
    local delay = math.random(9000, 11000)
    schedule(guid, "mindflay", delay, function()
        mindFlayTick(creature, guid)
    end)
end

-- C++ EVENT_SWARM: DoCastAOE + Talk(SAY_SWARM 3); 5s init, no
-- repeat (C++ does not reschedule it).
local function swarmTick(creature, guid)
    -- Same UpdateAI CASTING gates; one-shot arm defers 500ms
    -- rather than dropping (the event stays queued in C++).
    if creature:IsCasting() then
        schedule(guid, "swarm", 500, function()
            swarmTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SWARM)
    creature:Talk(SAY_SWARM)
end

-- C++ EVENT_FRENZY :282-286 — DoCastSelf(SPELL_FRENZY 28747) +
-- DoCastAOE(SPELL_CURSE_OF_FATIGUE 52592) rendered as self-cast, 1s
-- init, 15s repeat.
local function frenzyTick(creature, guid)
    -- Same UpdateAI CASTING gates (EVENT_FRENZY executes in the
    -- scheduler loop): 500ms defer, no repeat re-arm.
    if creature:IsCasting() then
        schedule(guid, "frenzy", 500, function()
            frenzyTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FRENZY)
    creature:CastSpell(creature, SPELL_CURSE_OF_FATIGUE)
    schedule(guid, "frenzy", 15000, function()
        frenzyTick(creature, guid)
    end)
end

-- C++ boss_krik_thirAI::UpdateAI :258-262 — HealthBelowPct(10) sets
-- _hadFrenzy and schedules EVENT_FRENZY. The per-tick check reads
-- post-damage health, so the event-9 hook computes it explicitly
-- (the halion HealthBelowPctDamaged precedent). C++ Reset() clears
-- _hadFrenzy (:154); leave-combat / died lead back through Reset in
-- the Go engine, so the latch is cleared on all three.
local hadFrenzy = {}

local function krikThirDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    if hadFrenzy[guid] then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    -- UpdateAI arm (boss_krikthir_the_gatewatcher.cpp:258): HealthBelowPct(10)
    -- reads post-damage health per tick and never fires on the killing blow;
    -- project post-damage health and guard lethal hits (nefarian form).
    local postDamage = math.max(health - damage, 0)
    if postDamage > 0 and postDamage < math.floor(maxHealth * 10 / 100) then
        hadFrenzy[guid] = true
        schedule(guid, "frenzy", 1000, function()
            frenzyTick(creature, guid)
        end)
    end
end


-- C++ _watchersActive / _petsInCombat, keyed by boss GUID (Reset
-- clears both; JustEngagedWith clears _petsInCombat only).
local bossMachine = {}

local function machineState(guid)
    local st = bossMachine[guid]
    if not st then
        st = { active = 0, pets = false }
        bossMachine[guid] = st
    end
    return st
end

-- C++ EVENT_SEND_GROUP (boss_krikthir_the_gatewatcher.cpp:265-268):
-- DoCastAOE(SUBBOSS_AGGRO_TRIGGER, triggered), 70s repeat.
local function sendGroupTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "sendgroup", 500, function()
            sendGroupTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SUBBOSS_AGGRO_TRIGGER, true)
    schedule(guid, "sendgroup", 70000, function()
        sendGroupTick(creature, guid)
    end)
end

-- C++ boss_krik_thirAI::DoAction (:214-251).
local function krikThirDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    local st = machineState(guid)
    if action == ACTION_GASHRA_DIED or action == ACTION_NARJIL_DIED
        or action == ACTION_SILTHIK_DIED then
        -- Zero active watchers means something is wrong -> evade;
        -- else decrement, and when the last one dies send the next
        -- group 5s out instead of waiting for the 70s repeat.
        if st.active == 0 then
            creature:EnterEvadeMode()
            return
        end
        st.active = st.active - 1
        if st.active == 0 then
            schedule(guid, "sendgroup", 5000, function()
                sendGroupTick(creature, guid)
            end)
        end
    elseif action == ACTION_WATCHER_ENGAGED then
        st.active = st.active + 1
    elseif action == ACTION_PET_ENGAGED then
        -- First pet engage only, and never while the boss is already
        -- in combat; Talk(SAY_AGGRO 0) is the real C++ aggro yell.
        if st.pets or creature:IsInCombat() then
            return
        end
        st.pets = true
        creature:Talk(SAY_AGGRO)
        schedule(guid, "sendgroup", 70000, function()
            sendGroupTick(creature, guid)
        end)
    elseif action == ACTION_PET_EVADE then
        creature:EnterEvadeMode()
    end
end

-- In-tree random-target bridge (anomalus spark / faerlina
-- precedent): alive threat-list targets, player-only on request.
local function randomTarget(creature, playersOnly)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() and (not playersOnly or t:GetObjectType() == "Player") then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- One rotation arm: mode "self" = DoCastSelf, "victim" =
-- DoCastVictim, "random" = DoCast(SelectTarget Random, no player
-- gate), "randomPlayer" = DoCast(SelectTarget Random, player-only).
local function watcherArmTick(creature, guid, arm)
    if creature:IsCasting() then
        schedule(guid, arm.key, 500, function()
            watcherArmTick(creature, guid, arm)
        end)
        return
    end
    if arm.mode == "self" then
        creature:CastSpell(creature, arm.spell)
    elseif arm.mode == "victim" then
        creature:CastSpell(nil, arm.spell)
    else
        local target = randomTarget(creature, arm.mode == "randomPlayer")
        if target then
            creature:CastSpell(target, arm.spell)
        end
    end
    schedule(guid, arm.key, math.random(arm.repMin, arm.repMax), function()
        watcherArmTick(creature, guid, arm)
    end)
end

-- C++ _JustEngagedWith inits + UpdateAI repeats, per watcher
-- (gashra :425-428/:446-460; narjil :501-504/:521-541; silthik
-- :580-583/:601-621).
local watcherRotations = {
    [ENTRY_WATCHER_GASHRA] = {
        { key = "enrage", spell = SPELL_ENRAGE, mode = "self", initMin = 3000, initMax = 5000, repMin = 12000, repMax = 20000 },
        { key = "webwrap", spell = SPELL_WEB_WRAP, mode = "random", initMin = 16000, initMax = 19000, repMin = 13000, repMax = 19000 },
        { key = "bite", spell = SPELL_INFECTED_BITE, mode = "victim", initMin = 7000, initMax = 11000, repMin = 23000, repMax = 27000 },
    },
    [ENTRY_WATCHER_NARJIL] = {
        { key = "webs", spell = SPELL_BLINDING_WEBS, mode = "victim", initMin = 13000, initMax = 18000, repMin = 23000, repMax = 27000 },
        { key = "webwrap", spell = SPELL_WEB_WRAP, mode = "randomPlayer", initMin = 3000, initMax = 5000, repMin = 13000, repMax = 19000 },
        { key = "bite", spell = SPELL_INFECTED_BITE, mode = "victim", initMin = 7000, initMax = 11000, repMin = 20000, repMax = 25000 },
    },
    [ENTRY_WATCHER_SILTHIK] = {
        { key = "spray", spell = SPELL_POISON_SPRAY, mode = "victim", initMin = 16000, initMax = 19000, repMin = 13000, repMax = 19000 },
        { key = "webwrap", spell = SPELL_WEB_WRAP, mode = "randomPlayer", initMin = 7000, initMax = 11000, repMin = 13000, repMax = 17000 },
        { key = "bite", spell = SPELL_INFECTED_BITE, mode = "victim", initMin = 3000, initMax = 5000, repMin = 20000, repMax = 24000 },
    },
}

-- C++ _isWatcher latch (:330-336) + passive group leg (:338-352):
-- each fires once per watcher lifetime (never re-armed; the
-- REACT_AGGRESSIVE flip after first engage blocks repeats).
local watcherEngaged = {}

local function makeWatcherEnterCombat(arms)
    return function(event, creature)
        local guid = creature:GetGUIDLow()
        if not watcherEngaged[guid] then
            watcherEngaged[guid] = true
            local summoner = creature:GetSummonerUnit()
            if summoner then
                summoner:DoAction(ACTION_WATCHER_ENGAGED)
                summoner:DoAction(ACTION_PET_ENGAGED)
            end
        end
        for _, arm in ipairs(arms) do
            schedule(guid, arm.key, math.random(arm.initMin, arm.initMax), function()
                watcherArmTick(creature, guid, arm)
            end)
        end
    end
end

local function watcherLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ watcher JustDied (:430-435 / :508-513 / :586-591): relay to
-- the boss only while it is alive.
local function makeWatcherDied(action)
    return function(event, creature, killer)
        local guid = creature:GetGUIDLow()
        cancelTimers(guid)
        watcherEngaged[guid] = nil
        local krikthir = creature:GetInstanceCreature(DATA_KRIKTHIR)
        if krikthir and krikthir:IsAlive() then
            krikthir:DoAction(action)
        end
    end
end

local function krikThirEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    machineState(guid).pets = false
    schedule(guid, "mindflay", math.random(1000, 3000), function()
        mindFlayTick(creature, guid)
    end)
    schedule(guid, "swarm", 5000, function()
        swarmTick(creature, guid)
    end)
    -- BossAI::JustEngagedWith base arms, handler tail (C++ base call
    -- is last, after the schedules); react-state/summons.DoZone legs out.
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
end

local function krikThirLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    hadFrenzy[guid] = nil
end

local function krikThirTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function krikThirDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    hadFrenzy[guid] = nil
    creature:Talk(SAY_DEATH)
end

local function krikThirReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    hadFrenzy[guid] = nil
    bossMachine[guid] = nil
end

RegisterCreatureEvent(ENTRY_KRIK_THIR, 1, krikThirEnterCombat)
RegisterCreatureEvent(ENTRY_KRIK_THIR, 2, krikThirLeaveCombat)
RegisterCreatureEvent(ENTRY_KRIK_THIR, 3, krikThirTargetDied)
RegisterCreatureEvent(ENTRY_KRIK_THIR, 4, krikThirDied)
RegisterCreatureEvent(ENTRY_KRIK_THIR, 9, krikThirDamageTaken)
RegisterCreatureEvent(ENTRY_KRIK_THIR, 23, krikThirReset)

RegisterCreatureEvent(ENTRY_KRIK_THIR, 38, krikThirDoAction)
RegisterCreatureEvent(ENTRY_WATCHER_GASHRA, 1, makeWatcherEnterCombat(watcherRotations[ENTRY_WATCHER_GASHRA]))
RegisterCreatureEvent(ENTRY_WATCHER_GASHRA, 2, watcherLeaveCombat)
RegisterCreatureEvent(ENTRY_WATCHER_GASHRA, 4, makeWatcherDied(ACTION_GASHRA_DIED))
RegisterCreatureEvent(ENTRY_WATCHER_GASHRA, 23, watcherLeaveCombat)
RegisterCreatureEvent(ENTRY_WATCHER_NARJIL, 1, makeWatcherEnterCombat(watcherRotations[ENTRY_WATCHER_NARJIL]))
RegisterCreatureEvent(ENTRY_WATCHER_NARJIL, 2, watcherLeaveCombat)
RegisterCreatureEvent(ENTRY_WATCHER_NARJIL, 4, makeWatcherDied(ACTION_NARJIL_DIED))
RegisterCreatureEvent(ENTRY_WATCHER_NARJIL, 23, watcherLeaveCombat)
RegisterCreatureEvent(ENTRY_WATCHER_SILTHIK, 1, makeWatcherEnterCombat(watcherRotations[ENTRY_WATCHER_SILTHIK]))
RegisterCreatureEvent(ENTRY_WATCHER_SILTHIK, 2, watcherLeaveCombat)
RegisterCreatureEvent(ENTRY_WATCHER_SILTHIK, 4, makeWatcherDied(ACTION_SILTHIK_DIED))
RegisterCreatureEvent(ENTRY_WATCHER_SILTHIK, 23, watcherLeaveCombat)
