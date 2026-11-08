-- Rotface (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_rotface.cpp
-- (813 lines incl. license; 4 CreatureScripts
-- (boss_rotface (BossAI, DATA_ROTFACE = 5),
-- npc_little_ooze (ScriptedAI), npc_big_ooze (ScriptedAI),
-- npc_precious_icc (ScriptedAI)) + 10 SpellScripts
-- (spell_rotface_ooze_flood, spell_rotface_mutated_infection,
-- spell_rotface_little_ooze_combine, spell_rotface_large_ooze_combine,
-- spell_rotface_large_ooze_buff_combine,
-- spell_rotface_unstable_ooze_explosion_init,
-- spell_rotface_unstable_ooze_explosion,
-- spell_rotface_unstable_ooze_explosion_suicide,
-- spell_rotface_vile_gas_trigger, spell_rotface_slime_spray)
-- + 1 AuraScript (spell_rotface_mutated_infection_aura); all
-- registered from inside AddSC_boss_rotface(); loader decl 176 /
-- call 371 per northrend_script_loader.cpp — the SIXTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_festergut() (call 370); the call
-- after it is AddSC_boss_professor_putricide() (call 372) —
-- verified from the loader this run; the checkpoint sequence
-- (boss_festergut -> boss_rotface) is followed).
-- Entry: 36627 Rotface (icecrown_citadel.h NPC_ROTFACE, line
-- 227; icecrown_citadel.h DATA_ROTFACE = 5, line 79;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_ROTFACE, line 241) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_rotface" hits boss_rotface.cpp (+ the loader
-- decl/call lines) only; this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No rotface lua
-- existed.
--
-- DOCUMENTED-BLOCKED (no registration beyond entry 36627):
-- boss_rotface UpdateAI — Talk(EMOTE_SLIME_SPRAY 2) rides the
-- EVENT_SLIME_SPRAY scheduler leg — PORTED 2026-10-25 (SummonCreature
-- + GetAITargets skip-first pool; see onSlimeSpray below).
-- PORTED (2026-10-13): EVENT_MUTATED_INFECTION —
-- DoCastAOE(SPELL_MUTATED_INFECTION 69674) non-triggered as
-- self-cast, 14s init on the lua-local hasten machine
-- (EVENT_HASTEN_INFECTIONS 90s ticks, -2s per tick, min 6s —
-- vexallus local-machine precedent); PORTED (2026-10-17):
-- EVENT_VILE_GAS — DoCastAOE(SPELL_VILE_GAS_TRIGGER 72285)
-- non-triggered as self-cast, 22s..27s init / 30s..35s re-arm,
-- heroic-gated via creature:IsHeroic()
-- (engine/world/lua_creature_events.go:260) — the 2026-10-14 "no
-- IsHeroic bridge" claim was stale.
-- boss_rotface SpellHitTarget — Talk(SAY_SLIME_SPRAY 3) rides the
-- SPELL_SLIME_SPRAY case of the SpellHitTarget switch — PORTED
-- (rotfaceSpellHitTarget, event 15): the C++ arm Talks once per hit
-- target of the spray AoE, while Go's self-cast path carries only
-- the explicit target (boss_ai.go:1181 hitTargets = {targetGUID} +
-- chain jumps only; area/cone effects are inert there), so the Go
-- fire carries target=self once per spray cast. The leg is
-- target-independent (Talk takes no target), so the single yell is
-- the faithful observable form — the gruul player-gated contrast,
-- where the target itself was the leg and the arm stayed out.
-- Event 15 itself is live (boss_ai.go:1904, grobbulus precedent).
-- boss_rotface JustEngagedWith instance legs —
-- instance->CheckRequiredBosses / DoCastSpellOnPlayers /
-- professor->AI()->DoAction(ACTION_ROTFACE_COMBAT) stays out — ev38 dispatch
-- bridged and sender rotface JustEngagedWith IS ported (boss_rotface.cpp:136),
-- but receiver arm ACTION_ROTFACE_COMBAT (boss_professor_putricide.cpp:440-471)
-- is the unported PHASE_ROTFACE machine
-- (SetPhase/MovePoint/REACT_PASSIVE/dummy-GUID init); no portable leg for a
-- krick relay; DoZoneInCombat (:138)
-- is PORTED 2026-10-23 via the live bridge (C++ arm order — after
-- Talk, before the proc self-cast); JustDied leg
-- professor->AI()->DoAction(ACTION_ROTFACE_DEATH) — Talk leg
-- PORTED 2026-11-03 (rotfaceJustDied 4500ms professor_death_talk
-- timer, festergut precedent; EnterEvadeMode stays out, no bridge);
-- JustDied's DoRemoveAurasDueToSpellOnPlayers(
-- MUTATED_INFECTION) is PORTED (2026-10-23) via GetPlayersInWorld +
-- mapId/instanceId filter + player:RemoveAura (the moroes
-- precedent — the stale "no instance bridge" claim is retracted);
-- JustReachedHome
-- instance->SetBossState(DATA_ROTFACE, FAIL) PORTED 2026-11-06 via
-- creature:SetBossState on ev24 OnReachHome (the marrowgar ev24
-- precedent); SetData(DATA_OOZE_DANCE_ACHIEVEMENT) PORTED 2026-11-08
-- (tick 457) via the shared instanceData oozeDanceEligible key
-- (marrowgar bonedEligible precedent). EnterEvadeMode
-- cross-AI professor EnterEvadeMode — no bridge; JustSummoned
-- cross-AI professor CastSpell leg (VILE_GAS_H) — no bridge.
-- JustSummoned latch PORTED 2026-11-03 (rotfaceJustSummoned,
-- event 19): summons latch, despawned from died/reset/leave; the
-- vile-gas stalker never spawns in Lua, so only the ooze-spray
-- stalker latch leg is live via the ported slime-spray summon site.
-- npc_precious_icc::JustDied —
-- rotface->AI()->Talk(SAY_PRECIOUS_DIES 0) is cross-AI Talk with no
-- bridge (the festergut stinky precedent); Talk(
-- EMOTE_PRECIOUS_ZOMBIES 0) rides the EVENT_SUMMON_ZOMBIES
-- scheduler leg (no-timer-bridge). npc_precious_icc has zero
-- bridgeable arms — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent). Its entry constant is
-- DB-side/script-name bound (no NPC_PRECIOUS constant in the ICC
-- script tree).
-- npc_little_ooze (36897, Wowhead WotLK npc=36897 — verified
-- 2026-11-05 via browser; abilities 69774/69750 + Merge match C++)
-- / npc_big_ooze (36899, Wowhead WotLK npc=36899 — same verification;
-- abilities 69774/69760/69558 + Unstable Ooze Explosion match C++) —
-- PORTED 2026-11-05 (littleOozeSummoned / bigOozeSummoned ev22 +
-- stickyOozeTick + littleOozeDied / bigOozeDied ev4 + bigOozeDoAction
-- ev38): IsSummonedBy self-cast auras (triggered) + 5s sticky init /
-- 15s re-arm + little AddThreat(summoner, 500000) + big Rotface
-- summons latch + big DoAction sticky-cancel + JustDied despawns.
-- LATENT: the producers are unported Spell/AuraScripts (little: the
-- mutated-infection aura OnRemove summon; big: Ooze Merge 69889 via
-- the combine SpellScripts), so ev22/ev38 fire only when those paths
-- land (krick precedent); the three combine SpellScripts themselves
-- are PORTED 2026-11-08 (tick 457) via ev5 RegisterSpellEvent
-- (below, dormant until Spell.dbc ships) — the explosion chain
-- (init/suicide/trigger-missile) is PORTED (tick 458 init/suicide,
-- tick 459 trigger-missile on the new veto bridge); the tick-458
-- suicide binding is corrected this tick (71441, not 69832).
-- npc_precious_icc — NOT registered, same entry-block (no
-- NPC_PRECIOUS constant in-tree): blocked arms EVENT_DECIMATE
-- DoCastVictim(71123) non-triggered / EVENT_MORTAL_WOUND
-- DoCastVictim(71127) non-triggered {3s,7s} init (:345-348,
-- same signatures as Stinky); EVENT_SUMMON_ZOMBIES 11x DoCastSelf(
-- SPELL_AWAKEN_PLAGUED_ZOMBIES 71159) non-triggered OUT (summon
-- leg, :394-400); JustDied cross-AI Talk(SAY_PRECIOUS_DIES 0)
-- stays (no cross-AI bridge).
-- Re-audit 2026-10-14 then 2026-10-17: EVENT_VILE_GAS DoCastAOE(
-- SPELL_VILE_GAS_TRIGGER 72285) non-triggered, 30s..35s re-arm
-- (:220-222) is pure but HEROIC-GATED (scheduled only
-- `if (IsHeroic())` :117-118). The 10-14 claim of "no IsHeroic
-- bridge" was STALE — creature:IsHeroic() is live on the
-- event-handler surface (engine/world/lua_creature_events.go:260,
-- boss_blood_prince_council.lua uses it) — so the arm is now
-- PORTED with the heroic gate (this run); porting unconditionally
-- would cast on normal too (unfaithful), so the gate is required.
-- The remaining 7 spell scripts: 7/7 PORTED 2026-11-08 (ticks
-- 458-459) via RegisterSpellEvent/RegisterAuraEvent (below, dormant
-- until Spell.dbc ships) — ooze_flood (ev7 + ev5),
-- mutated_infection (ev7 + ev3 AfterHit Talk), explosion_init (ev5),
-- vile_gas_trigger (ev7 + ev5), slime_spray (ev2),
-- explosion_suicide (aura ev1, bound 71441), explosion (ev4, bound
-- 69832). Bindings verified this tick against spell_script_names:
-- (69832,'spell_rotface_unstable_ooze_explosion'),
-- (69839,'spell_rotface_unstable_ooze_explosion_init'),
-- (71441,'spell_rotface_unstable_ooze_explosion_suicide_aura') —
-- the tick-458 explosion-on-69839 framing was wrong (69839 carries
-- the init's force-cast + the 71441 trigger; the trigger-missile
-- leg lives on 69832). 69839 effect 0 RESOLVED tick 461: 140
-- FORCE_CAST -> 69840 (Wago SpellEffect 3.4.4 + C++ :662 registers
-- the init on EFFECT_0 SPELL_EFFECT_FORCE_CAST; the 163 hypothesis
-- is refuted — 163 is EffectNULL in 3.3.5) — vetoed by the init
-- leg's PreventHitEffect(0), so 69840 never fires; effect 1 is 64
-- TRIGGER_SPELL -> 71441, covered by the Go default arm. The 3 combine scripts are PORTED
-- 2026-11-08 (tick 457) via ev5 RegisterSpellEvent (below): the
-- buff-combine Talk(EMOTE_UNSTABLE_2..4 0..2) +
-- Talk(EMOTE_UNSTABLE_EXPLOSION 3) + rotface
-- Talk(SAY_UNSTABLE_EXPLOSION 5) (:597-:618) and the large-combine
-- rotface Talk(SAY_UNSTABLE_EXPLOSION 5) (:557) ride the ev5
-- handlers now, dormant until Spell.dbc ships.
-- spell_rotface_mutated_infection_aura PORTED 2026-11-08 (tick 425,
-- below; dormant on the Spell.dbc CalcValue trigger id).
-- All 14 Talk() calls in the file accounted for (14 =
-- SAY_PRECIOUS_DIES 0 (1, cross-AI from npc_precious_icc JustDied)
-- + SAY_AGGRO 1 (1) + EMOTE_SLIME_SPRAY 2 (1, scheduler) +
-- SAY_SLIME_SPRAY 3 (1, SpellHitTarget) + SAY_UNSTABLE_EXPLOSION 5
-- (2, cross-AI from the large_ooze_combine + large_ooze_buff_combine
-- ev5 handlers, PORTED 2026-11-08 tick 457) + EMOTE_PRECIOUS_ZOMBIES 0 (1, scheduler) +
-- SAY_KILL 6 (1) + SAY_DEATH 8 (1) +
-- EMOTE_MUTATED_INFECTION 9 (1, SpellScript) +
-- EMOTE_UNSTABLE_2..4 / EMOTE_UNSTABLE_EXPLOSION 0..3 (4,
-- ev5 handlers, PORTED 2026-11-08 tick 457); SAY_BERSERK 7 is declared in the C++ Texts enum
-- but never used in a Talk() call).

local ENTRY_ROTFACE = 36627
local ENTRY_PROFESSOR_PUTRICIDE = 36678

local DATA_ROTFACE = 5 -- icecrown_citadel.h:79
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3

local SAY_AGGRO = 1
local SAY_SLIME_SPRAY = 3
local SAY_UNSTABLE_EXPLOSION = 5 -- boss_rotface.cpp:38
local SAY_KILL = 6
local SAY_DEATH = 8
-- Professor Putricide Talk id for the rotface-death relay (boss_professor_putricide.cpp:43).
local SAY_PROFESSOR_ROTFACE_DEATH = 3

local SPELL_GREEN_ABOMINATION = 70001
local SPELL_MUTATED_INFECTION = 69674
-- MUTATED_INFECTION RAID_MODE(69674, 71224, 73022, 73023)
-- (boss_rotface.cpp:85): only one variant can be present per instance,
-- so the strip pool checks all four (the twin_valkyr essence-pool
-- convention).
local MUTATED_INFECTION_AURAS = { 69674, 71224, 73022, 73023 }
local SPELL_VILE_GAS_TRIGGER = 72285 -- boss_rotface.cpp:82
local SPELL_SLIME_SPRAY = 69508 -- boss_rotface.cpp:55
local NPC_OOZE_SPRAY_STALKER = 37986 -- icecrown_citadel.h:228
local ENTRY_LITTLE_OOZE = 36897 -- Wowhead WotLK npc=36897, verified 2026-11-05 (browser)
local ENTRY_BIG_OOZE = 36899 -- Wowhead WotLK npc=36899, verified 2026-11-05 (browser)
local SPELL_LITTLE_OOZE_COMBINE = 69537 -- boss_rotface.cpp:60
local SPELL_LARGE_OOZE_COMBINE = 69552 -- boss_rotface.cpp:61
local SPELL_LARGE_OOZE_BUFF_COMBINE = 69611 -- boss_rotface.cpp:62
local SPELL_WEAK_RADIATING_OOZE = 69750 -- boss_rotface.cpp:64
local SPELL_RADIATING_OOZE = 69760 -- boss_rotface.cpp:65
local SPELL_UNSTABLE_OOZE = 69558 -- boss_rotface.cpp:66
local SPELL_STICKY_OOZE = 69774 -- boss_rotface.cpp:69
local SPELL_OOZE_MERGE = 69889 -- boss_rotface.cpp:63
local SPELL_UNSTABLE_OOZE_EXPLOSION = 69839 -- boss_rotface.cpp:68
local SPELL_VERTEX_COLOR_PINK = 53213 -- boss_rotface.cpp:71
local SPELL_VERTEX_COLOR_BRIGHT_RED = 69844 -- boss_rotface.cpp:72
local SPELL_VERTEX_COLOR_DARK_RED = 44773 -- boss_rotface.cpp:73
local EVENT_STICKY_OOZE = 8 -- boss_rotface.cpp:100
local EMOTE_SLIME_SPRAY = 2
local EMOTE_UNSTABLE_2 = 0 -- boss_rotface.cpp:44
local EMOTE_UNSTABLE_3 = 1 -- boss_rotface.cpp:45
local EMOTE_UNSTABLE_4 = 2 -- boss_rotface.cpp:46
local EMOTE_UNSTABLE_EXPLOSION = 3 -- boss_rotface.cpp:47
local EMOTE_MUTATED_INFECTION = 9 -- boss_rotface.cpp:48
-- Ooze-flood variants the professor cycles (boss_professor_putricide.cpp:168).
local OOZE_FLOOD_SPELLS = { 69782, 69796, 69798, 69801 }
local NPC_UNSTABLE_EXPLOSION_STALKER = 38107 -- icecrown_citadel.h:230
local SPELL_UNSTABLE_OOZE_EXPLOSION_TRIGGER = 69832 -- boss_rotface.cpp:70
local SPELL_UNSTABLE_OOZE_EXPLOSION_SUICIDE = 71441 -- spell_script_names (71441,'spell_rotface_unstable_ooze_explosion_suicide_aura')
local SPELL_VILE_GAS_TRIGGER_SUMMON = 72287 -- boss_rotface.cpp:57
local SPELL_GREEN_BLIGHT_RESIDUE = 72145 -- icecrown_citadel.h:42
local QUEST_RESIDUE_RENDEZVOUS_10 = 24873 -- icecrown_citadel.h:521
local QUEST_RESIDUE_RENDEZVOUS_25 = 24878 -- icecrown_citadel.h:522
local QUEST_STATUS_INCOMPLETE = 3 -- lua_player.go GetQuestStatus: NONE 0, COMPLETE 1, INCOMPLETE 3

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

local function cancelKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

local infection = {}

-- C++ EVENT_SLIME_SPRAY (:200-208): SelectTarget(Random, 1, 0.0f,
-- playerOnly) — offset 1 pops the current victim (UnitAI.h
-- non-distance list is victim-first), not the threat-sorted head —
-- then DoSummon(NPC_OOZE_SPRAY_STALKER, *target, 8s,
-- TEMPSUMMON_TIMED_DESPAWN), Talk(EMOTE_SLIME_SPRAY),
-- DoCastSelf(SPELL_SLIME_SPRAY) non-triggered; re-arm 20s
-- unconditional (the ScheduleEvent sits outside the target-pick
-- if). Pool via GetAITargets with the victim excluded by GUID,
-- Player + alive filter; summonType 2 = TIMED_DESPAWN, 8000ms.
local function onSlimeSpray(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :193 (+ in-loop :228, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "slime_spray", 500, function() onSlimeSpray(creature, guid) end)
        return
    end
    local pool = {}
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local aliveSeen = 0
    for _, unit in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = unit:GetGUID() == victimGuid
        elseif not unit:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and unit:GetObjectType() == "Player" and not unit:IsDead() then
            pool[#pool + 1] = unit
        end
    end
    if #pool > 0 then
        local target = pool[math.random(#pool)]
        creature:SummonCreature(NPC_OOZE_SPRAY_STALKER, target:GetX(),
            target:GetY(), target:GetZ(), target:GetO(), 2, 8000)
        creature:Talk(EMOTE_SLIME_SPRAY)
        creature:CastSpell(creature, SPELL_SLIME_SPRAY)
    end
    schedule(guid, "slime_spray", 20000, function() onSlimeSpray(creature, guid) end)
end

-- C++ EVENT_MUTATED_INFECTION: DoCastAOE(SPELL_MUTATED_INFECTION)
-- non-triggered -> self-cast (selin_fireheart/mr_smite DoCastAOE
-- convention); 14s init, re-arm on the infectionCooldown machine
-- (:115-116/:216-218).
local function onMutatedInfection(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :193 (+ in-loop :228, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "mutated_infection", 500, function() onMutatedInfection(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_MUTATED_INFECTION)
    local st = infection[guid]
    schedule(guid, "mutated_infection", st.cooldown, function() onMutatedInfection(creature, guid) end)
end

-- C++ EVENT_HASTEN_INFECTIONS machine (:209-215): infectionStage++ < 4
-- -> infectionCooldown -= 2s (14s -> 12s -> 10s -> 8s -> 6s), re-arm 90s;
-- 5th tick is a no-op — lua-local state (vexallus local-machine precedent).
local function onHastenInfections(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :193 (+ in-loop :228, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "hasten", 500, function() onHastenInfections(creature, guid) end)
        return
    end
    local st = infection[guid]
    if st.stage < 4 then
        st.stage = st.stage + 1
        st.cooldown = st.cooldown - 2000
        schedule(guid, "hasten", 90000, function() onHastenInfections(creature, guid) end)
    end
end

-- C++ EVENT_VILE_GAS (:219-222): DoCastAOE(SPELL_VILE_GAS_TRIGGER)
-- non-triggered -> self-cast (selin_fireheart/mr_smite convention);
-- heroic-gated: Reset schedules it only `if (IsHeroic())` (:117-118),
-- so the Lua arm gates on creature:IsHeroic()
-- (engine/world/lua_creature_events.go:260 — the 2026-10-14 "no
-- IsHeroic bridge" claim was stale); 22s..27s init, 30s..35s re-arm.
local function onVileGas(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :193 (+ in-loop :228, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "vile_gas", 500, function() onVileGas(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_VILE_GAS_TRIGGER)
    schedule(guid, "vile_gas", {30000, 35000}, function() onVileGas(creature, guid) end)
end

-- C++ boss_rotface::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent; the
-- CheckRequiredBosses evade branch (:126-130) has no bridge.
local function rotfaceJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    infection[guid] = nil
    creature:Talk(SAY_AGGRO)
    -- C++ :138 — me-form DoZoneInCombat after Talk(SAY_AGGRO) / professor
    -- DoAction, before the DoCastSelf(proc) leg (C++ arm order).
    creature:DoZoneInCombat()
    -- C++ :139 — DoCastSelf(SPELL_GREEN_ABOMINATION_HITTIN__YA_PROC, true)
    -- (triggered self-cast).
    creature:CastSpell(creature, SPELL_GREEN_ABOMINATION, true)
    infection[guid] = { stage = 0, cooldown = 14000 }
    schedule(guid, "mutated_infection", 14000, function() onMutatedInfection(creature, guid) end)
    schedule(guid, "slime_spray", 20000, function() onSlimeSpray(creature, guid) end)
    schedule(guid, "hasten", 90000, function() onHastenInfections(creature, guid) end)
    if creature:IsHeroic() then
        schedule(guid, "vile_gas", math.random(22000, 27000), function() onVileGas(creature, guid) end)
    end
end

-- C++ Reset/JustDied _Reset()/_JustDied() timer machinery —
-- cancel/re-arm on combat events (the maiden convention).
-- C++ JustSummoned (boss_rotface.cpp:177-184): vile-gas stalker ->
-- professor CastSpell (instance-blocked, stays out); every summon ->
-- summons.Summon latch. Table defined above its first caller
-- (halazzi/skadi idiom); the latch feeds Reset _Reset (:111-113) +
-- JustDied _JustDied (:142-145) + leave DespawnAll. The ooze-spray
-- stalker arrives via a direct Lua summon (slime-spray site); the big
-- ooze latches via its ev22 handler's Rotface grid sweep (C++
-- IsSummonedBy JustSummoned leg); little oozes are never latched
-- (C++ latches only big oozes — little ones are player-summoned).
local rotfaceSummons = {}

local function despawnRotfaceSummons(guid)
    local latched = rotfaceSummons[guid]
    if latched then
        for _, summon in ipairs(latched) do
            if summon then
                -- A despawned creature's C++ EventMap dies with it;
                -- cancel its Lua timers too (big-ooze sticky ticks).
                cancelTimers(summon:GetGUID())
                summon:Despawn()
            end
        end
        rotfaceSummons[guid] = nil
    end
end

local function rotfaceJustSummoned(event, creature, summon)
    if not summon then
        return
    end
    local guid = creature:GetGUID()
    local latched = rotfaceSummons[guid]
    if not latched then
        latched = {}
        rotfaceSummons[guid] = latched
    end
    latched[#latched + 1] = summon
end

local function rotfaceLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    despawnRotfaceSummons(guid)
    cancelTimers(guid)
    infection[guid] = nil
end

local function rotfaceReset(event, creature)
    local guid = creature:GetGUID()
    -- C++ Reset() :111 — _Reset() first: SetBossState NOT_STARTED
    -- only when the stored state != DONE (BossAI::_Reset,
    -- ScriptedCreature.cpp:499-509; the falric precedent — ev23
    -- fires on spawn/death/evade too, and a DONE boss must stay
    -- DONE). PORTED 2026-11-06 via creature:SetBossState.
    if creature:GetBossState(DATA_ROTFACE) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_ROTFACE, BOSS_STATE_NOT_STARTED)
    end
    despawnRotfaceSummons(guid)
    cancelTimers(guid)
    infection[guid] = nil
end

-- C++ instance OnCreatureCreate latching
-- (instance_icecrown_citadel.cpp:241): Rotface's GUID under
-- DATA_ROTFACE (5) in the instance guid store, so the combine
-- SpellScripts below resolve it via creature:GetInstanceCreature
-- (the illidari-council precedent; ev5 ON_SPAWN fires on first
-- spawn + respawn, instance_guid_store.go).
local function rotfaceSpawned(event, creature)
    creature:SetGuidData(DATA_ROTFACE)
end

-- C++ boss_rotface::JustReachedHome (:151-156): _JustReachedHome()
-- (setActive(false), unmodeled) then SetBossState(FAIL) PORTED
-- 2026-11-06 via creature:SetBossState on ev24 OnReachHome (the
-- marrowgar ev24 precedent). The SetData(DATA_OOZE_DANCE_ACHIEVEMENT
-- 15, true) reset leg PORTS 2026-11-08 (tick 457) via the shared
-- instanceData oozeDanceEligible key (marrowgar bonedEligible
-- precedent), written as 1/0 — InstanceDataNumber reads numbers only.
local function rotfaceReachHome(event, creature)
    creature:SetBossState(DATA_ROTFACE, BOSS_STATE_FAIL)
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.oozeDanceEligible = 1
        end
    end
end

-- C++ boss_rotface::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function rotfaceKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- Cross-AI DoAction relay (2026-11-03 strip): C++ rotface JustDied
-- reaches Professor Putricide via instance->GetGuidData; Lua has no
-- instance bridge, so the relay sweeps the 100yd grid for the
-- professor (the festergut relayProfessor precedent — exactly one
-- Putricide exists, so the first alive match is the C++ target).
local function relayProfessor(creature, fn)
    for _, professor in ipairs(creature:GetCreaturesInGrid(ENTRY_PROFESSOR_PUTRICIDE, 100)) do
        if professor:IsAlive() then
            fn(professor)
            return
        end
    end
end

-- C++ boss_rotface::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent; _JustDied()
-- resets the EventMap, so the self-re-arming mutated/hasten/vile
-- timers must be cancelled here (CreateLuaEvent timers have no death
-- auto-cancel — the devourer/festergut death-cancel precedent).
local function rotfaceJustDied(event, creature, killer)
    local guid = creature:GetGUID()
    despawnRotfaceSummons(guid)
    -- C++ JustDied (:141-148): instance->
    -- DoRemoveAurasDueToSpellOnPlayers(MUTATED_INFECTION) is the
    -- FIRST arm — via GetPlayersInWorld + mapId/instanceId filter +
    -- player:RemoveAura (the moroes precedent; the stale "no
    -- instance bridge" claim is retracted); the professor
    -- DoAction(ACTION_ROTFACE_DEATH) leg is ported below (Talk leg
    -- only; EnterEvadeMode + phase state stay out, no bridge).
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            for _, aura in ipairs(MUTATED_INFECTION_AURAS) do
                p:RemoveAura(aura)
            end
        end
    end
    -- C++ JustDied (:142) — _JustDied() sets DONE, after the aura
    -- strip above and before Talk (C++ arm order). PORTED 2026-11-06
    -- via creature:SetBossState.
    creature:SetBossState(DATA_ROTFACE, BOSS_STATE_DONE)
    cancelTimers(guid)
    infection[guid] = nil
    creature:Talk(SAY_DEATH)
    -- C++ :148 + boss_professor_putricide.cpp:479-480,590-592 —
    -- professor DoAction(ACTION_ROTFACE_DEATH) schedules
    -- EVENT_ROTFACE_DIES at 4500ms: Talk(SAY_ROTFACE_DEATH 3) +
    -- EnterEvadeMode (no bridge — stays out). The Talk leg rides a
    -- 4500ms timer on the rotface GUID, after cancelTimers above
    -- (the festergut death-talk precedent).
    schedule(guid, "professor_death_talk", 4500, function()
        relayProfessor(creature, function(professor)
            professor:Talk(SAY_PROFESSOR_ROTFACE_DEATH)
        end)
    end)
end

-- C++ boss_rotface SpellHitTarget (:171-174): SPELL_SLIME_SPRAY hit
-- -> Talk(SAY_SLIME_SPRAY). Fires once per spray cast in Go (the
-- self-cast hit list is {self}); see the header note above.
local function rotfaceSpellHitTarget(event, creature, target, spellId)
    if spellId == SPELL_SLIME_SPRAY then
        creature:Talk(SAY_SLIME_SPRAY)
    end
end

-- C++ sticky-ooze UpdateAI arm (little :268-271, big :322-324):
-- DoCastVictim(SPELL_STICKY_OOZE 69774), re-arm 15s. The C++
-- UpdateVictim gate freezes the timer without a victim — the 5s
-- re-poll below is the no-victim twin (kelthuzad blood-tap
-- precedent). No IsCasting gate exists in either ooze UpdateAI, so
-- no defer. Shared by both ooze sizes (same arm, same timers).
local function stickyOozeTick(creature, guid)
    if not creature:GetVictim() then
        schedule(guid, "sticky_ooze", 5000, function()
            stickyOozeTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_STICKY_OOZE)
    schedule(guid, "sticky_ooze", 15000, function()
        stickyOozeTick(creature, guid)
    end)
end

-- C++ npc_little_ooze::IsSummonedBy (boss_rotface.cpp:245-256):
-- DoCastSelf(LITTLE_OOZE_COMBINE 69537 + WEAK_RADIATING_OOZE 69750 +
-- GREEN_ABOMINATION_HITTIN__YA_PROC 70001, all triggered) +
-- AddThreat(summoner, 500000) + EVENT_STICKY_OOZE 5s init. LATENT:
-- the producer is the spell_rotface_mutated_infection_aura OnRemove
-- (target casts the EFFECT_2 summon spell on aura removal; PORTED
-- 2026-11-08 tick 425, dormant on the Spell.dbc CalcValue trigger
-- id), so ev22 fires only when that path lands (krick precedent).
-- The summoner is the formerly infected player.
local function littleOozeSummoned(event, creature, summoner)
    creature:CastSpell(creature, SPELL_LITTLE_OOZE_COMBINE, true)
    creature:CastSpell(creature, SPELL_WEAK_RADIATING_OOZE, true)
    creature:CastSpell(creature, SPELL_GREEN_ABOMINATION, true)
    if summoner then
        creature:AddThreat(summoner, 500000)
    end
    local guid = creature:GetGUID()
    schedule(guid, "sticky_ooze", 5000, function()
        stickyOozeTick(creature, guid)
    end)
end

-- C++ npc_little_ooze::JustDied (:258-261): DespawnOrUnsummon.
local function littleOozeDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Despawn()
end

-- C++ npc_big_ooze::IsSummonedBy (boss_rotface.cpp:286-298):
-- DoCastSelf(LARGE_OOZE_COMBINE 69552 + LARGE_OOZE_BUFF_COMBINE 69611
-- + RADIATING_OOZE 69760 + UNSTABLE_OOZE 69558 + GREEN_ABOMINATION
-- 70001, all triggered) + EVENT_STICKY_OOZE 5s init + register in
-- Rotface's summons (rotface->AI()->JustSummoned). LATENT: the
-- producer is the unported Ooze Merge 69889 via the combine
-- SpellScripts, so ev22 fires only when that path lands (krick
-- precedent). The Rotface latch mirrors C++ via the relayProfessor
-- grid sweep (same file): the big ooze joins rotfaceSummons so
-- Reset/JustDied/leave DespawnAll covers it.
local function bigOozeSummoned(event, creature, summoner)
    creature:CastSpell(creature, SPELL_LARGE_OOZE_COMBINE, true)
    creature:CastSpell(creature, SPELL_LARGE_OOZE_BUFF_COMBINE, true)
    creature:CastSpell(creature, SPELL_RADIATING_OOZE, true)
    creature:CastSpell(creature, SPELL_UNSTABLE_OOZE, true)
    creature:CastSpell(creature, SPELL_GREEN_ABOMINATION, true)
    local guid = creature:GetGUID()
    schedule(guid, "sticky_ooze", 5000, function()
        stickyOozeTick(creature, guid)
    end)
    for _, boss in ipairs(creature:GetCreaturesInGrid(ENTRY_ROTFACE, 100)) do
        if boss:IsAlive() then
            local bossGuid = boss:GetGUID()
            local latched = rotfaceSummons[bossGuid]
            if not latched then
                latched = {}
                rotfaceSummons[bossGuid] = latched
            end
            latched[#latched + 1] = creature
            return
        end
    end
end

-- C++ npc_big_ooze::DoAction (boss_rotface.cpp:306-309): action ==
-- EVENT_STICKY_OOZE (8) cancels the sticky event. LATENT: the sender
-- is the unported large_ooze_combine SpellScript (krick precedent).
local function bigOozeDoAction(event, creature, action)
    if action ~= EVENT_STICKY_OOZE then
        return
    end
    cancelKey(creature:GetGUID(), "sticky_ooze")
end

-- C++ npc_big_ooze::JustDied (:301-305): SummonedCreatureDespawn
-- (remove from Rotface's summons) + DespawnOrUnsummon.
local function bigOozeDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    for _, latched in pairs(rotfaceSummons) do
        for i, summon in ipairs(latched) do
            if summon and summon:GetGUID() == guid then
                table.remove(latched, i)
                break
            end
        end
    end
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_ROTFACE, 1, rotfaceJustEngagedWith)
RegisterCreatureEvent(ENTRY_ROTFACE, 2, rotfaceLeaveCombat)
RegisterCreatureEvent(ENTRY_ROTFACE, 3, rotfaceKilledUnit)
RegisterCreatureEvent(ENTRY_ROTFACE, 4, rotfaceJustDied)
RegisterCreatureEvent(ENTRY_ROTFACE, 5, rotfaceSpawned)
RegisterCreatureEvent(ENTRY_ROTFACE, 15, rotfaceSpellHitTarget)
RegisterCreatureEvent(ENTRY_ROTFACE, 19, rotfaceJustSummoned)
RegisterCreatureEvent(ENTRY_ROTFACE, 23, rotfaceReset)
RegisterCreatureEvent(ENTRY_ROTFACE, 24, rotfaceReachHome)
RegisterCreatureEvent(ENTRY_LITTLE_OOZE, 4, littleOozeDied)
RegisterCreatureEvent(ENTRY_LITTLE_OOZE, 22, littleOozeSummoned)
RegisterCreatureEvent(ENTRY_BIG_OOZE, 4, bigOozeDied)
RegisterCreatureEvent(ENTRY_BIG_OOZE, 22, bigOozeSummoned)
RegisterCreatureEvent(ENTRY_BIG_OOZE, 38, bigOozeDoAction)

-- C++ spell_rotface_mutated_infection_aura::HandleEffectRemove
-- (boss_rotface.cpp:485-504): AfterEffectRemove, EFFECT_0
-- SPELL_AURA_PERIODIC_DAMAGE, AURA_EFFECT_HANDLE_REAL. No remove-mode
-- guard in C++: fires on ANY remove (expire, dispel, death). The
-- trigger spell id is Effects[EFFECT_2].CalcValue() of the aura's own
-- spell, read via aura:GetSpellEffectCalcValue (lua_aura.go) — it reads
-- 0 with no Spell.dbc in the Go repo, so the ~= 0 guard no-ops until
-- Spell.dbc lands. PORTED 2026-11-08 (tick 425).
-- Deviation: C++ passes {aurEff, GetCasterGUID()} (original caster) —
-- Lua CastSpell has no original-caster slot, so the cast attributes to
-- the target. Triggered flag kept; on the player session path the
-- triggered arg is ignored but the cast fires.
local function mutatedInfectionRemove(event, aura, target)
    local triggerSpell = aura:GetSpellEffectCalcValue(aura:GetAuraId(), 2)
    if triggerSpell ~= 0 then
        target:CastSpell(target, triggerSpell, true)
    end
end

for _, auraId in ipairs(MUTATED_INFECTION_AURAS) do
    RegisterAuraEvent(auraId, 3, mutatedInfectionRemove)
end

-- Shared explosion branch of the two large-ooze combine scripts
-- (boss_rotface.cpp:549-563 / :610-624): remove both combine auras,
-- clear the ooze-dance latch (only when Rotface resolves via the
-- instance guid store — the C++ SetData sits inside the
-- `if (rotface)` block), rotface Talk if alive, sticky-cancel
-- DoAction (the big-ooze ev38 receiver, above), triggered
-- explosion cast.
local function explodeUnstableOoze(caster)
    caster:RemoveAura(SPELL_LARGE_OOZE_BUFF_COMBINE)
    caster:RemoveAura(SPELL_LARGE_OOZE_COMBINE)
    local rotface = caster:GetInstanceCreature(DATA_ROTFACE)
    if rotface ~= nil then
        local mapObj = caster:GetMap()
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data then
                data.oozeDanceEligible = 0
            end
        end
        if rotface:IsAlive() then
            rotface:Talk(SAY_UNSTABLE_EXPLOSION)
        end
    end
    caster:DoAction(EVENT_STICKY_OOZE)
    caster:CastSpell(caster, SPELL_UNSTABLE_OOZE_EXPLOSION, true)
end

-- C++ spell_rotface_little_ooze_combine::HandleScript
-- (boss_rotface.cpp:506-524): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT. Guard: hit creature alive + caster
-- non-nil; caster + hit each RemoveAurasDueToSpell(LITTLE_OOZE_COMBINE
-- 69537) — creature:RemoveAura is the twin (marrowgar precedent);
-- hit casts OOZE_MERGE (69889) on the caster (triggered); both
-- DespawnOrUnsummon. DORMANT until Spell.dbc ships: the combine cast
-- is DBC-gated (creature path needs spellKnown), like the razorscale
-- summon_iron_dwarves leg (tick 456).
local function littleOozeCombine(event, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if not target or target:GetObjectType() ~= "Creature" or target:IsDead() then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:RemoveAura(SPELL_LITTLE_OOZE_COMBINE)
    target:RemoveAura(SPELL_LITTLE_OOZE_COMBINE)
    target:CastSpell(caster, SPELL_OOZE_MERGE, true)
    target:DespawnOrUnsummon()
    caster:DespawnOrUnsummon()
end

-- C++ spell_rotface_large_ooze_combine::HandleScript
-- (boss_rotface.cpp:529-571): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT. Caster's UNSTABLE_OOZE (69558) aura
-- gains the target's stacks (ModStackAmount twin: Get + Set) or +1
-- when the target carries none; at >= 5 the explosion branch runs
-- (explodeUnstableOoze, above); the hit ooze despawns
-- unconditionally (outside the aura if, :565). DORMANT until
-- Spell.dbc ships (as above).
local function largeOozeCombine(event, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if not target or target:GetObjectType() ~= "Creature" or target:IsDead() then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local unstable = caster:GetAura(SPELL_UNSTABLE_OOZE)
    if unstable ~= nil then
        local targetAura = target:GetAura(SPELL_UNSTABLE_OOZE)
        local add = 1
        if targetAura ~= nil then
            add = targetAura:GetStackAmount()
        end
        local stacks = unstable:GetStackAmount() + add
        unstable:SetStackAmount(stacks)
        if stacks >= 5 then
            explodeUnstableOoze(caster)
        end
    end
    target:DespawnOrUnsummon()
end

-- C++ spell_rotface_large_ooze_buff_combine::HandleScript
-- (boss_rotface.cpp:574-636): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT; Load() requires a unit caster (the
-- creature ev5 path only fires for creature casters). New stack =
-- old + 1; the 2/3/4 rungs Talk the emote (caster as the $N target)
-- + cast the vertex-color spell (triggered); rung 5 Talks the
-- explosion emote then runs the shared explosion branch
-- (explodeUnstableOoze, above); the hit ooze despawns
-- unconditionally (outside the aura if, :630). DORMANT until
-- Spell.dbc ships (as above).
local function largeOozeBuffCombine(event, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if not target or target:GetObjectType() ~= "Creature" or target:IsDead() then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local unstable = caster:GetAura(SPELL_UNSTABLE_OOZE)
    if unstable ~= nil then
        local newStack = unstable:GetStackAmount() + 1
        unstable:SetStackAmount(newStack)
        if newStack == 2 then
            caster:Talk(EMOTE_UNSTABLE_2, caster)
            caster:CastSpell(caster, SPELL_VERTEX_COLOR_PINK, true)
        elseif newStack == 3 then
            caster:Talk(EMOTE_UNSTABLE_3, caster)
            caster:CastSpell(caster, SPELL_VERTEX_COLOR_BRIGHT_RED, true)
        elseif newStack == 4 then
            caster:Talk(EMOTE_UNSTABLE_4, caster)
            caster:CastSpell(caster, SPELL_VERTEX_COLOR_DARK_RED, true)
        elseif newStack == 5 then
            caster:Talk(EMOTE_UNSTABLE_EXPLOSION)
            explodeUnstableOoze(caster)
        end
    end
    target:DespawnOrUnsummon()
end

RegisterSpellEvent(SPELL_LITTLE_OOZE_COMBINE, 5, littleOozeCombine)
RegisterSpellEvent(SPELL_LARGE_OOZE_COMBINE, 5, largeOozeCombine)
RegisterSpellEvent(SPELL_LARGE_OOZE_BUFF_COMBINE, 5, largeOozeBuffCombine)

-- 3D distance between two units. No GetDistance bridge exists
-- anywhere in engine/ (verified this tick), so the reach
-- subtraction in C++ WorldObject::GetDistance (Object.cpp:1175-1179)
-- is not replicated: orderings match except near-ties within
-- combined combat reach, and the 5.0yd melee/ranged split below can
-- flip a target sitting within reach of the boundary.
local function dist3d(a, b)
    local dx = a:GetX() - b:GetX()
    local dy = a:GetY() - b:GetY()
    local dz = a:GetZ() - b:GetZ()
    return math.sqrt(dx * dx + dy * dy + dz * dz)
end

-- C++ spell_rotface_ooze_flood::FilterTargets
-- (boss_rotface.cpp:437-447): OnObjectAreaTargetSelect EFFECT_0
-- TARGET_UNIT_SRC_AREA_ENTRY. Sort by distance from the caster,
-- keep the 5 nearest (resize pops the farthest), then drop the
-- nearest until 2 remain — the 2 farthest of the 5 nearest. Go
-- hands the effect's per-effect partition
-- (lua_spell.go fireSpellTargetSelects); DORMANT until Spell.dbc
-- ships (the flood cast is DBC-gated, professor DoAction leg
-- unported).
local function oozeFloodFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local list = targets:GetTargets()
    table.sort(list, function(a, b) return dist3d(a, caster) < dist3d(b, caster) end)
    while #list > 5 do
        targets:Remove(list[#list])
        table.remove(list)
    end
    while #list > 2 do
        targets:Remove(list[1])
        table.remove(list, 1)
    end
end

-- C++ spell_rotface_ooze_flood::HandleScript
-- (boss_rotface.cpp:419-435): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT. The hit unit casts the effect value
-- (spell:GetEffectValue(0), the DBC CalcValue twin — 0 until
-- Spell.dbc ships, fail-closed) on the farthest same-entry
-- creature within 12.5yd. Deviation: C++ passes the original
-- caster's GUID (the professor); Lua CastSpell has no
-- original-caster slot, so the cast attributes to the hit unit
-- (same class as the tick-425 aura-port deviation).
local function oozeFloodScript(event, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if not target or target:GetObjectType() ~= "Creature" then
        return
    end
    local effectValue = spell:GetEffectValue(0)
    if effectValue == 0 then
        return
    end
    local triggers = target:GetCreaturesInGrid(target:GetEntry(), 12.5)
    if #triggers == 0 then
        return
    end
    table.sort(triggers, function(a, b) return dist3d(a, target) < dist3d(b, target) end)
    target:CastSpell(triggers[#triggers], effectValue)
end

-- C++ spell_rotface_mutated_infection::FilterTargets
-- (boss_rotface.cpp:458-471): OnObjectAreaTargetSelect EFFECT_ALL
-- TARGET_UNIT_SRC_AREA_ENEMY. Drop every target already carrying
-- the aura (UnitAuraCheck, spell's own id), then keep one random
-- survivor. No effectIndex gate: C++ registers EFFECT_ALL and the
-- Go fire hands each selecting effect its own list, so the filter
-- runs per effect exactly like C++. DORMANT until Spell.dbc ships.
local function mutatedInfectionFilter(event, spell, effectIndex, targets)
    local spellId = spell:GetEntry()
    for _, target in ipairs(targets:GetTargets()) do
        if target:HasAura(spellId) then
            targets:Remove(target)
        end
    end
    local kept = targets:GetTargets()
    if #kept == 0 then
        return
    end
    local pick = kept[math.random(#kept)]
    local pickGuid = pick:GetGUID()
    for _, target in ipairs(kept) do
        if target:GetGUID() ~= pickGuid then
            targets:Remove(target)
        end
    end
end

-- C++ spell_rotface_mutated_infection::NotifyTargets
-- (boss_rotface.cpp:473-478): AfterHit — the caster creature's AI
-- Talks EMOTE_MUTATED_INFECTION (9) at the hit unit. DORMANT
-- until Spell.dbc ships (same cast as the filter).
local function mutatedInfectionNotify(event, spell, target)
    local caster = spell:GetCaster()
    if caster == nil or target == nil then
        return
    end
    caster:Talk(EMOTE_MUTATED_INFECTION, target)
end

-- C++ spell_rotface_unstable_ooze_explosion_init::HandleCast
-- (boss_rotface.cpp:645-658): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_FORCE_CAST. Summon the explosion stalker
-- (38107, TEMPSUMMON_TIMED_DESPAWN = type 2, 1min) at the hit
-- unit's position, then the caster casts the trigger spell
-- (69832) on it, triggered. PreventHitEffect(0) vetoes the default
-- force-cast via the tick-459 bridge (lua_spell.go
-- spellEffectPreventState; the veto collapses to the whole effect —
-- faithful here, the cast targets the ooze itself). DORMANT until
-- Spell.dbc ships.
local function unstableOozeExplosionInit(event, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    spell:PreventHitEffect(0)
    if not target then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local dummy = caster:SummonCreature(NPC_UNSTABLE_EXPLOSION_STALKER,
        target:GetX(), target:GetY(), target:GetZ(), 0, 2, 60000)
    if dummy ~= nil then
        caster:CastSpell(dummy, SPELL_UNSTABLE_OOZE_EXPLOSION_TRIGGER, true)
    end
end

-- C++ spell_rotface_vile_gas_trigger::FilterTargets
-- (boss_rotface.cpp:718-760): OnObjectAreaTargetSelect EFFECT_0
-- TARGET_UNIT_SRC_AREA_ENEMY. Distance-sorted split into melee
-- (< 5.0yd) and ranged; top the ranged pool up to 8 (25-man) / 3
-- (10-man) with random melee picks, then RandomResize the ranged
-- pool to 3 / 1 — the kept targets are the ranged pool only.
-- DORMANT until Spell.dbc ships.
local function vileGasTriggerFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local is25 = caster:Is25ManRaid()
    local list = targets:GetTargets()
    table.sort(list, function(a, b) return dist3d(a, caster) < dist3d(b, caster) end)
    local ranged = {}
    local melee = {}
    for _, target in ipairs(list) do
        if dist3d(target, caster) < 5.0 then
            melee[#melee + 1] = target
        else
            ranged[#ranged + 1] = target
        end
    end
    local minTargets = 3
    if is25 then
        minTargets = 8
    end
    while #ranged < minTargets and #melee > 0 do
        local i = math.random(#melee)
        ranged[#ranged + 1] = melee[i]
        table.remove(melee, i)
    end
    local keep = 1
    if is25 then
        keep = 3
    end
    while #ranged > keep do
        table.remove(ranged, math.random(#ranged))
    end
    local keepGuids = {}
    for _, target in ipairs(ranged) do
        keepGuids[target:GetGUID()] = true
    end
    for _, target in ipairs(targets:GetTargets()) do
        if not keepGuids[target:GetGUID()] then
            targets:Remove(target)
        end
    end
end

-- C++ spell_rotface_vile_gas_trigger::HandleDummy
-- (boss_rotface.cpp:766-770): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_DUMMY. The caster casts the summon spell (72287)
-- on the hit unit, NON-triggered (C++ has no triggered flag).
-- PreventHitDefaultEffect is a no-op here: SPELL_EFFECT_DUMMY has
-- no default arm on the creature path (boss_ai.go:3527 skips dummy
-- effects). DORMANT until Spell.dbc ships.
local function vileGasTriggerDummy(event, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if not target then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(target, SPELL_VILE_GAS_TRIGGER_SUMMON)
end

-- C++ spell_rotface_slime_spray::HandleResidue
-- (boss_rotface.cpp:773-787): OnHit. Players only; skip bearers of
-- the residue aura (72145) and anyone whose Residue Rendezvous
-- quest (24873 10-man / 24878 25-man) is not INCOMPLETE; the rest
-- cast the residue on themselves, triggered. DORMANT until
-- Spell.dbc ships — and the self-cast hit list carries only the
-- caster until area/cone hit resolution lands (boss_ai.go:1181),
-- so the player arm is unreachable before that.
local function slimeSprayResidue(event, spell, target)
    if not target or target:GetObjectType() ~= "Player" then
        return
    end
    if target:HasAura(SPELL_GREEN_BLIGHT_RESIDUE) then
        return
    end
    local caster = spell:GetCaster()
    local questId = QUEST_RESIDUE_RENDEZVOUS_10
    if caster ~= nil and caster:Is25ManRaid() then
        questId = QUEST_RESIDUE_RENDEZVOUS_25
    end
    if target:GetQuestStatus(questId) ~= QUEST_STATUS_INCOMPLETE then
        return
    end
    target:CastSpell(target, SPELL_GREEN_BLIGHT_RESIDUE, true)
end

-- C++ spell_rotface_unstable_ooze_explosion_suicide::DespawnSelf
-- (boss_rotface.cpp:693-709): AuraScript OnEffectPeriodic EFFECT_2
-- SPELL_AURA_PERIODIC_TRIGGER_SPELL, bound to 71441
-- (spell_script_names (71441,
-- 'spell_rotface_unstable_ooze_explosion_suicide_aura'); 71441's
-- effects are Pacify & Silence, Root, Periodic trigger spell).
-- Creature targets only: strip all
-- auras, hide, despawn in 60s. `return true` IS the
-- PreventDefaultAction: fireAuraPeriodicTick (lua_aura.go) treats
-- a true return as the veto and skips the default tick. DORMANT
-- until Spell.dbc ships (the 71441 aura application rides 69839
-- effect 1, DBC-gated).
local function unstableOozeExplosionSuicide(event, aura, target)
    if target ~= nil and target:GetObjectType() == "Creature" then
        target:RemoveAllAuras()
        target:SetVisible(false)
        target:DespawnOrUnsummon(60000)
    end
    return true
end

-- C++ spell_rotface_unstable_ooze_explosion::CheckTarget
-- (boss_rotface.cpp:666-685): OnEffectHit EFFECT_0
-- SPELL_EFFECT_TRIGGER_MISSILE, bound to 69832 (spell_script_names
-- (69832,'spell_rotface_unstable_ooze_explosion'); 69832's only DBC
-- effect is Trigger Missile -> 69839). PreventHitDefaultEffect the
-- missile, then Rotface casts the effect's TriggerSpell at the
-- explosion dest, the ooze as original caster. PORTED 2026-11-08
-- (tick 459) on the new veto bridge (lua_spell.go:
-- spell:PreventHitDefaultEffect + spell:GetExplTargetDest +
-- spell:GetTriggerSpell). DORMANT until Spell.dbc ships.
-- Deviation: Lua CastSpellAoF has no original-caster slot
-- (tick-425 class), so the cast attributes to Rotface instead of
-- the ooze.
local function unstableOozeExplosion(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    spell:PreventHitDefaultEffect(0)
    local x, y, z = spell:GetExplTargetDest()
    if x == nil then
        return
    end
    local triggerSpell = spell:GetTriggerSpell(0)
    if triggerSpell == 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local rotface = caster:GetInstanceCreature(DATA_ROTFACE)
    if rotface == nil then
        return
    end
    rotface:CastSpellAoF(x, y, z, triggerSpell, true)
end

for _, spellId in ipairs(OOZE_FLOOD_SPELLS) do
    RegisterSpellEvent(spellId, 7, oozeFloodFilter)
    RegisterSpellEvent(spellId, 5, oozeFloodScript)
end
for _, auraId in ipairs(MUTATED_INFECTION_AURAS) do
    RegisterSpellEvent(auraId, 7, mutatedInfectionFilter)
    RegisterSpellEvent(auraId, 3, mutatedInfectionNotify)
end
RegisterSpellEvent(SPELL_UNSTABLE_OOZE_EXPLOSION, 5, unstableOozeExplosionInit)
RegisterSpellEvent(SPELL_UNSTABLE_OOZE_EXPLOSION_TRIGGER, 4, unstableOozeExplosion)
RegisterSpellEvent(SPELL_VILE_GAS_TRIGGER, 7, vileGasTriggerFilter)
RegisterSpellEvent(SPELL_VILE_GAS_TRIGGER, 5, vileGasTriggerDummy)
RegisterSpellEvent(SPELL_SLIME_SPRAY, 2, slimeSprayResidue)
RegisterAuraEvent(SPELL_UNSTABLE_OOZE_EXPLOSION_SUICIDE, 1, unstableOozeExplosionSuicide)
