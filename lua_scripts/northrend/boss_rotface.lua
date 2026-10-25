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
-- SPELL_SLIME_SPRAY case of the SpellHitTarget switch — stays out:
-- the C++ arm Talks once per hit target of the spray AoE, but Go's
-- self-cast path carries only the explicit target, so the ev15
-- target correspondence is unverified (event 15 itself is live —
-- boss_ai.go:1904, grobbulus precedent fires per hit target).
-- boss_rotface JustEngagedWith instance legs —
-- instance->CheckRequiredBosses / DoCastSpellOnPlayers /
-- professor->AI()->DoAction(ACTION_ROTFACE_COMBAT) (no-DoAction
-- bridge; the krick ACTION_OUTRO precedent); DoZoneInCombat (:138)
-- is PORTED 2026-10-23 via the live bridge (C++ arm order — after
-- Talk, before the proc self-cast); JustDied leg
-- professor->AI()->DoAction(ACTION_ROTFACE_DEATH) — no DoAction
-- bridge; JustDied's DoRemoveAurasDueToSpellOnPlayers(
-- MUTATED_INFECTION) is PORTED (2026-10-23) via GetPlayersInWorld +
-- mapId/instanceId filter + player:RemoveAura (the moroes
-- precedent — the stale "no instance bridge" claim is retracted);
-- JustReachedHome
-- instance->SetBossState(DATA_ROTFACE, FAIL) + SetData(
-- DATA_OOZE_DANCE_ACHIEVEMENT) — no instance bridge; EnterEvadeMode
-- cross-AI professor EnterEvadeMode — no bridge; JustSummoned
-- cross-AI professor CastSpell leg (VILE_GAS_H) — no bridge.
-- npc_precious_icc::JustDied —
-- rotface->AI()->Talk(SAY_PRECIOUS_DIES 0) is cross-AI Talk with no
-- bridge (the festergut stinky precedent); Talk(
-- EMOTE_PRECIOUS_ZOMBIES 0) rides the EVENT_SUMMON_ZOMBIES
-- scheduler leg (no-timer-bridge). npc_precious_icc has zero
-- bridgeable arms — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent). Its entry constant is
-- DB-side/script-name bound (no NPC_PRECIOUS constant in the ICC
-- script tree).
-- npc_little_ooze / npc_big_ooze — zero Talk lines — NOT
-- registered; bridgeable-in-principle but ENTRY-BLOCKED (the
-- pit_of_saron flamebearer precedent): no NPC_LITTLE_OOZE /
-- NPC_BIG_OOZE entry constants anywhere in-tree (this clone has
-- no TDB; RegisterCreatureEvent needs an entry ID). The blocked
-- arms: EVENT_STICKY_OOZE DoCastVictim(SPELL_STICKY_OOZE 69774)
-- non-triggered, 5s init / 15s re-arm (:268-271 / :322-324);
-- IsSummonedBy triggered self-cast aura arms (:249-251 / :287-291);
-- the combine / unstable machinery is SpellScript/DoAction
-- territory and stays out.
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
-- The 10 spell scripts join the no-SpellScript-bridge queue (the
-- boss_moragg optic-link precedent): spell_rotface_mutated_infection
-- Talk(EMOTE_MUTATED_INFECTION 9) is cross-AI Talk riding a
-- SpellScript (NotifyTargets); spell_rotface_large_ooze_buff_combine
-- Talk(EMOTE_UNSTABLE_2..4 0..2) + Talk(EMOTE_UNSTABLE_EXPLOSION 3)
-- + rotface->AI()->Talk(SAY_UNSTABLE_EXPLOSION 5) (:597-:618) and
-- spell_rotface_large_ooze_combine rotface->AI()->Talk(
-- SAY_UNSTABLE_EXPLOSION 5) (:557) are caster / cross-AI Talk riding
-- SpellScripts; spell_rotface_unstable_ooze_explosion_init carries
-- zero Talk calls; the remaining 7 join the
-- same queue on their caster / targeting legs.
-- spell_rotface_mutated_infection_aura joins the no-AuraScript-bridge
-- queue.
-- All 14 Talk() calls in the file accounted for (14 =
-- SAY_PRECIOUS_DIES 0 (1, cross-AI from npc_precious_icc JustDied)
-- + SAY_AGGRO 1 (1) + EMOTE_SLIME_SPRAY 2 (1, scheduler) +
-- SAY_SLIME_SPRAY 3 (1, SpellHitTarget) + SAY_UNSTABLE_EXPLOSION 5
-- (2, cross-AI from the large_ooze_combine + large_ooze_buff_combine
-- SpellScripts) + EMOTE_PRECIOUS_ZOMBIES 0 (1, scheduler) +
-- SAY_KILL 6 (1) + SAY_DEATH 8 (1) +
-- EMOTE_MUTATED_INFECTION 9 (1, SpellScript) +
-- EMOTE_UNSTABLE_2..4 / EMOTE_UNSTABLE_EXPLOSION 0..3 (4,
-- SpellScript); SAY_BERSERK 7 is declared in the C++ Texts enum
-- but never used in a Talk() call).

local ENTRY_ROTFACE = 36627

local SAY_AGGRO = 1
local SAY_KILL = 6
local SAY_DEATH = 8

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
local EMOTE_SLIME_SPRAY = 2

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

local infection = {}

-- C++ EVENT_SLIME_SPRAY (:200-208): SelectTarget(Random, 1, 0.0f,
-- playerOnly) — offset 1 skips the first MaxThreat-ordered entry
-- (UnitAI.h:162-174) — then DoSummon(NPC_OOZE_SPRAY_STALKER,
-- *target, 8s, TEMPSUMMON_TIMED_DESPAWN), Talk(EMOTE_SLIME_SPRAY),
-- DoCastSelf(SPELL_SLIME_SPRAY) non-triggered; re-arm 20s
-- unconditional (the ScheduleEvent sits outside the target-pick
-- if). Pool via GetAITargets (threat-sorted, SortedEntries) with
-- the head skipped, Player + alive filter (grobbulus/festergut
-- precedents); summonType 2 = TIMED_DESPAWN, 8000ms.
local function onSlimeSpray(creature, guid)
    local pool = {}
    local targets = creature:GetAITargets()
    for i = 2, #targets do
        local unit = targets[i]
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
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
    creature:CastSpell(creature, SPELL_MUTATED_INFECTION)
    local st = infection[guid]
    schedule(guid, "mutated_infection", st.cooldown, function() onMutatedInfection(creature, guid) end)
end

-- C++ EVENT_HASTEN_INFECTIONS machine (:209-215): infectionStage++ < 4
-- -> infectionCooldown -= 2s (14s -> 12s -> 10s -> 8s -> 6s), re-arm 90s;
-- 5th tick is a no-op — lua-local state (vexallus local-machine precedent).
local function onHastenInfections(creature, guid)
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
local function rotfaceLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    infection[guid] = nil
end

local function rotfaceReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    infection[guid] = nil
end

-- C++ boss_rotface::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function rotfaceKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_rotface::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent; _JustDied()
-- resets the EventMap, so the self-re-arming mutated/hasten/vile
-- timers must be cancelled here (CreateLuaEvent timers have no death
-- auto-cancel — the devourer/festergut death-cancel precedent).
local function rotfaceJustDied(event, creature, killer)
    local guid = creature:GetGUID()
    -- C++ JustDied (:141-148): instance->
    -- DoRemoveAurasDueToSpellOnPlayers(MUTATED_INFECTION) is the
    -- FIRST arm — via GetPlayersInWorld + mapId/instanceId filter +
    -- player:RemoveAura (the moroes precedent; the stale "no
    -- instance bridge" claim is retracted); the professor
    -- DoAction(ACTION_ROTFACE_DEATH) leg has no bridge.
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            for _, aura in ipairs(MUTATED_INFECTION_AURAS) do
                p:RemoveAura(aura)
            end
        end
    end
    cancelTimers(guid)
    infection[guid] = nil
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_ROTFACE, 1, rotfaceJustEngagedWith)
RegisterCreatureEvent(ENTRY_ROTFACE, 2, rotfaceLeaveCombat)
RegisterCreatureEvent(ENTRY_ROTFACE, 3, rotfaceKilledUnit)
RegisterCreatureEvent(ENTRY_ROTFACE, 4, rotfaceJustDied)
RegisterCreatureEvent(ENTRY_ROTFACE, 23, rotfaceReset)
