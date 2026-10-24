-- Bronjahm (Forge of Souls) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/ForgeOfSouls/boss_bronjahm.cpp
-- (411 lines incl. license; 8 scripts — boss_bronjahm (CreatureScript via
-- GetForgeOfSoulsAI (BossAI), DATA_BRONJAHM = 0); npc_corrupted_soul_fragment
-- (CreatureScript via GetForgeOfSoulsAI (ScriptedAI)); spell_bronjahm_magic_bane /
-- spell_bronjahm_consume_soul / spell_bronjahm_soulstorm_targeting
-- (SpellScriptLoaders); spell_bronjahm_soulstorm_visual (AuraScript,
-- registered twice: channel + visual); achievement_bronjahm_soul_power
-- (AchievementCriteriaScript); all registered from inside
-- AddSC_boss_bronjahm(); loader decl 157 / call 352 per
-- northrend_script_loader.cpp — the THIRD group of the "// Forge of Souls"
-- block in AddNorthrendScripts(), after AddSC_forge_of_souls() (call 351),
-- before AddSC_boss_devourer_of_souls() (call 353).
-- Entries: 36497 Bronjahm (forge_of_souls.h NPC_BRONJAHM, line 40) — the
-- CreatureScript ScriptName binding is DB-side as usual. DATA_BRONJAHM = 0
-- (forge_of_souls.h line 31). DATA_SOUL_POWER = 1.
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_bronjahm"
-- hits boss_bronjahm.cpp only (+ the loader decl/call lines).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent
-- ({min,max} table = randtime range); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
--
-- TIMER-PORT DOCTRINE (CreateLuaEvent is a real registered bridge —
-- engine/scripting/runtime.go:798; the maiden_of_virtue / moroes / curator /
-- nightbane convention): timers table keyed by creature GUID, cancelled on
-- Eluna events 2 (leave combat), 4 (death), 23 (reset); CastSpell(nil,
-- spellId) = victim-cast (bridge falls back to TargetGUID);
-- CastSpell(creature, spellId) = self-cast; TRIGGERED casts pass true as
-- the third CastSpell arg. The phase machine is a SIMPLE LATCH here
-- (DamageTaken flips a per-GUID flag; no summon lists, MoveFollow, or
-- DoAction cross-creature infrastructure), so it ports — the gothik
-- precedent would only block phase-gated arms riding unbridged machines.
--
-- PORTED ARMS (C++ line refs; all C++-exact for the modeled arms):
-- Reset schedule (:86-89) — on enter combat: SHADOW_BOLT 70043 at 2000ms
--   (C++ :87; re-arm :184 phase-1 2000ms, :178 phase-2 {1000,2000}ms),
--   MAGIC_BANE 68793 at {8000,20000}ms (C++ :88/:172 — DoCastAOE is
--   self-cast, re-arm :172 {8000,20000}ms both phases),
--   CORRUPT_SOUL 68839 at {25000,35000}ms phase-1 only (C++ :89/:193).
-- EVENT_SHADOW_BOLT (:174-185) — victim-cast 70043 (:177/:183); phase-1
--   casts only when the victim is outside melee range (:181-183).
-- EVENT_MAGIC_BANE (:170-173) — DoCastAOE(68793) self-cast, both phases.
-- EVENT_CORRUPT_SOUL (:187-194) — random player target
--   (SelectTarget Random position 1 playerOnly :188; approximated with a
--   random alive player like the maiden precedent), Talk(SAY_CORRUPT_SOUL 4)
--   (:190) + cast 68839 on the target (:191), phase-1 only (re-arm :193
--   PHASE_1-gated, so no fabricated phase-2 casts).
-- DamageTaken 30%-hp PHASE_1->PHASE_2 flip (:117-125) — simple latch:
--   event 9 flips the per-GUID phase flag (post-damage health via
--   (GetHealth()-damage), the moroes precedent), self-cast TELEPORT 68988
--   (:122), schedules EVENT_FEAR (:123 {12000,16000}ms phase-2) and the
--   EVENT_SOULSTORM one-shot (:124 100ms phase-2).
-- EVENT_FEAR (:200-203) — CastSpell(nil, 68950) victim-cast; re-arm
--   {8000,12000}ms phase-2 only (:202).
-- EVENT_SOULSTORM one-shot chain (:195-199) — Talk(SAY_SOUL_STORM 3) (:196),
--   triggered self-cast SOULSTORM_VISUAL 68870 (:197), non-triggered
--   self-cast SOULSTORM 68872 (:198).
-- JustEngagedWith (:98-103) — Talk(SAY_AGGRO 0) (:101) unconditional;
--   RemoveAurasDueToSpell(SPELL_SOULSTORM_CHANNEL 69008) (:102) via the
--   RemoveAura bridge (the arlokk precedent).
-- KilledUnit (:111-115) — Talk(SAY_SLAY 1) player-gated (:114).
-- JustDied (:105-109) — Talk(SAY_DEATH 2) (:108) unconditional.
--
-- DOCUMENTED-ONLY (no registration beyond entry 36497):
-- The ctor SPELL_SOULSTORM_CHANNEL 69008 pre-fight cast (:80) has no
--   ctor bridge; JustReachedHome's cast (:95) is ported below on event 24
--   (this tick — the old "no JustReachedHome bridge" note was stale:
--   OnReachHome fires at the same point as C++ HomeMovementGenerator::
--   DoFinalize). The removal on engage is ported, so the channel visual
--   clears exactly like C++.
-- The JustSummoned soul-fragment machine (:128-136) is PORTED as
--   bronjahmJustSummoned (event 19): REACT_PASSIVE :133 +
--   PURPLE_BANISH_VISUAL self-cast :135 (triggered); MoveFollow :134
--   has no motion bridge and stays out, so fragments still will not
--   follow the boss; summons.Summon has no Lua summon list.
-- GetData(DATA_SOUL_POWER) (:139-154) joins the no-GetData-bridge queue
--   (the boss_zuramat precedent); achievement_bronjahm_soul_power rides it
--   into the unmodeled-achievement queue.
-- npc_corrupted_soul_fragment (36535) is NOT registered: IsSummonedBy
--   hands off through instance->GetGuidData(DATA_BRONJAHM) (the
--   no-instance-GetGuidData queue) and MovementInform rides the
--   no-MovementInform bridge (:249 CastSpell(nullptr, SPELL_CONSUME_SOUL,
--   true) + DespawnOrUnsummon fire only on FOLLOW arrival) — zero
--   standalone bridgeable arms (the moroes/valithria add-registration rule).
-- The four SpellScript/AuraScript hooks (magic_bane mana-power damage
--   recalc :284, consume_soul script-effect redirect :311, soulstorm_visual
--   8-spell periodic ring :345/:350, soulstorm_targeting 10yd exclusion)
--   join the no-SpellScript / no-AuraScript bridge queues.
--
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts are
-- packet-visual), so timers fire unconditionally (the maiden precedent);
-- no threat model — corrupt-soul target is a random alive player with no
-- target at all when none qualify (Talk + cast stay inside the C++ :188
-- target-found leg); EVENT_FEAR's { SPELLVALUE_MAX_TARGETS, 1 } custom leg
-- (:201) has no bridge, so fear is not limited to 1 target and Lua's nil
-- falls back to the victim rather than C++'s self-centered AoE; phase-2
-- melee keeps swinging (C++ :212 gates DoMeleeAttackIfReady on phase-1;
-- the phase-2 melee stop has no bridge); IsWithinMeleeRange has no bridge
-- — 5 yd via IsWithinDist approximates the phase-1 shadow-bolt melee gate
-- (the kiljaeden precedent); DamageTaken fires pre-application and the C++ arm
-- (boss_bronjahm.cpp:117-118) reads the same pre-application health (Unit.cpp:701
-- runs the hook before the health reduction), so plain GetHealth() is exact;
-- !HealthAbovePct(30) is health <= floor(max*30/100), inclusive of exactly-30.

local ENTRY_BRONJAHM = 36497

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_SOUL_STORM = 3
local SAY_CORRUPT_SOUL = 4

local SPELL_MAGIC_BANE = 68793
local SPELL_SHADOW_BOLT = 70043
local SPELL_CORRUPT_SOUL = 68839
local SPELL_TELEPORT = 68988
local SPELL_FEAR = 68950
local SPELL_SOULSTORM = 68872
local SPELL_SOULSTORM_CHANNEL = 69008
local SPELL_SOULSTORM_VISUAL = 68870

local ENTRY_CORRUPTED_SOUL_FRAGMENT = 36535
local SPELL_PURPLE_BANISH_VISUAL = 68862
local REACT_PASSIVE = 0

local timers = {}
local phase2 = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

-- arlokk-style: re-scheduling a key replaces the pending timer, so each
-- named arm has exactly one live timer at a time.
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

local function initState(guid)
    cancelTimers(guid)
    phase2[guid] = nil
end

-- C++ :187 SelectTarget(Random, 1, 0.0f, true) — position=1 skips the
-- threat head, playerOnly=true; the netherspite victim-GUID exclusion
-- idiom approximates both.
local function randomPlayer(creature)
    local victim = creature:GetVictim()
    local victimGuid = victim ~= nil and victim:GetGUID() or nil
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetGUID() ~= victimGuid and p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ :170-173 EVENT_MAGIC_BANE: DoCastAOE(SPELL_MAGIC_S_BANE) is
-- self-cast (the DoCastAOE convention); re-arm {8000,20000}ms, both phases.
local function onMagicBane(creature, guid)
    creature:CastSpell(creature, SPELL_MAGIC_BANE)
    schedule(guid, "magicbane", {8000, 20000}, function() onMagicBane(creature, guid) end)
end

-- C++ :174-185 EVENT_SHADOW_BOLT: phase-2 is unconditional victim-cast
-- with {1000,2000}ms re-arm (:175-178); phase-1 casts only outside melee
-- range (:181-183, 5-yd approximation) with a 2000ms re-arm (:184).
local function onShadowBolt(creature, guid)
    if phase2[guid] then
        creature:CastSpell(nil, SPELL_SHADOW_BOLT)
        schedule(guid, "shadowbolt", {1000, 2000}, function() onShadowBolt(creature, guid) end)
    else
        local victim = creature:GetVictim()
        if victim and not creature:IsWithinDist(victim, 5) then
            creature:CastSpell(nil, SPELL_SHADOW_BOLT)
        end
        schedule(guid, "shadowbolt", 2000, function() onShadowBolt(creature, guid) end)
    end
end

-- C++ :187-194 EVENT_CORRUPT_SOUL (PHASE_1 only): random player target
-- (SelectTarget position=1 = threat head excluded, via randomPlayer),
-- Talk(SAY_CORRUPT_SOUL), cast 68839 on the target; re-arm {25000,35000}ms
-- phase-1-gated, so a phase-2 flip between fires kills the chain.
local function onCorruptSoul(creature, guid)
    if phase2[guid] then
        return
    end
    local target = randomPlayer(creature)
    if target then
        creature:Talk(SAY_CORRUPT_SOUL)
        creature:CastSpell(target, SPELL_CORRUPT_SOUL)
    end
    schedule(guid, "corruptsoul", {25000, 35000}, function() onCorruptSoul(creature, guid) end)
end

-- C++ :195-199 EVENT_SOULSTORM one-shot: Talk(SAY_SOUL_STORM),
-- triggered self-cast 68870, non-triggered self-cast 68872.
local function onSoulstorm(creature, guid)
    creature:Talk(SAY_SOUL_STORM)
    creature:CastSpell(creature, SPELL_SOULSTORM_VISUAL, true)
    creature:CastSpell(creature, SPELL_SOULSTORM)
end

-- C++ :200-203 EVENT_FEAR (PHASE_2 only): CastSpell(nullptr, SPELL_FEAR,
-- { SPELLVALUE_MAX_TARGETS, 1 }) — the nullptr runs the spell's implicit
-- random-enemy selection capped at 1 target, not a victim cast. The Go cast
-- path binds an explicit target GUID only, so nil would always fear the
-- tank; randomFearTarget restores the C++ pick (tank stays in the pool, per
-- the implicit targeting; the existing randomPlayer helper excludes the
-- victim for the Corrupt Soul SelectTarget(Random, 1) arm and does not
-- apply here). Re-arm {8000,12000}ms.
local function randomFearTarget(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onFear(creature, guid)
    if not phase2[guid] then
        return
    end
    local target = randomFearTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_FEAR)
    end
    schedule(guid, "fear", {8000, 12000}, function() onFear(creature, guid) end)
end

-- C++ :98-103 JustEngagedWith: Talk(SAY_AGGRO) unconditional; the
-- RemoveAurasDueToSpell(SOULSTORM_CHANNEL) pre-fight-channel removal via
-- the RemoveAura bridge. C++ :86-89 Reset schedule lands here.
local function bronjahmEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    initState(guid)
    creature:Talk(SAY_AGGRO)
    creature:RemoveAura(SPELL_SOULSTORM_CHANNEL)
    schedule(guid, "shadowbolt", 2000, function() onShadowBolt(creature, guid) end)
    schedule(guid, "magicbane", {8000, 20000}, function() onMagicBane(creature, guid) end)
    schedule(guid, "corruptsoul", {25000, 35000}, function() onCorruptSoul(creature, guid) end)
end

-- C++ :117-125 DamageTaken: PHASE_1->PHASE_2 flip when !HealthAbovePct(30)
-- is a simple latch (no summon lists, MoveFollow, or DoAction legs), so it
-- ports. C++ takes uint32& /*damage*/ (unnamed) — the check is on CURRENT
-- health, no damage projection (floored <=30 form, = !HealthAbovePct(30) exactly).
-- Self-cast TELEPORT 68988 (:122); schedule EVENT_FEAR {12000,16000}ms
-- (:123) and the EVENT_SOULSTORM one-shot at 100ms (:124), both phase-2.
local function bronjahmDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if phase2[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() <= math.floor(maxHealth * 30 / 100) then
        phase2[guid] = true
        creature:CastSpell(creature, SPELL_TELEPORT)
        schedule(guid, "fear", {12000, 16000}, function() onFear(creature, guid) end)
        schedule(guid, "soulstorm", 100, function() onSoulstorm(creature, guid) end)
    end
end

-- C++ :111-115 KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function bronjahmKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ :105-109 JustDied: Talk(SAY_DEATH) — unconditional; _JustDied()
-- (passthrough, no bridge) — the sjonnir JustDied-Talk precedent.
local function bronjahmJustDied(event, creature)
    initState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function bronjahmLeaveCombat(event, creature)
    initState(creature:GetGUID())
end

local function bronjahmReset(event, creature)
    initState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_BRONJAHM, 1, bronjahmEnterCombat)
RegisterCreatureEvent(ENTRY_BRONJAHM, 2, bronjahmLeaveCombat)
RegisterCreatureEvent(ENTRY_BRONJAHM, 3, bronjahmKilledUnit)
RegisterCreatureEvent(ENTRY_BRONJAHM, 4, bronjahmJustDied)
RegisterCreatureEvent(ENTRY_BRONJAHM, 9, bronjahmDamageTaken)
RegisterCreatureEvent(ENTRY_BRONJAHM, 23, bronjahmReset)

-- C++ boss_bronjahmAI::JustSummoned (boss_bronjahm.cpp:128-136):
-- entry NPC_CORRUPTED_SOUL_FRAGMENT 36535 -> summons.Summon (no Lua
-- summon list) + SetReactState(REACT_PASSIVE) + triggered self-cast
-- PURPLE_BANISH_VISUAL 68862. MoveFollow (:134) has no motion bridge
-- and stays out; the fragment IsSummonedBy instance-GetGuidData leg
-- stays out. Event 19 fires on the summoner for spell-effect summons
-- through the shared summonTempCreature funnel (boss_ai.go:1734),
-- so the ported CORRUPT_SOUL cast delivers this arm.
local function bronjahmJustSummoned(event, creature, summon)
    if summon:GetEntry() ~= ENTRY_CORRUPTED_SOUL_FRAGMENT then
        return
    end
    summon:SetReactState(REACT_PASSIVE)
    summon:CastSpell(summon, SPELL_PURPLE_BANISH_VISUAL, true)
end
RegisterCreatureEvent(ENTRY_BRONJAHM, 19, bronjahmJustSummoned)

-- C++ boss_bronjahmAI::JustReachedHome (:94-96): _JustReachedHome() is
-- instance bookkeeping (no bridge); DoCast(me, SPELL_SOULSTORM_CHANNEL,
-- true) is a TRIGGERED self-cast, ported via the Eluna third-arg
-- triggered convention. Event 24 fires after the home-move completes.
local function bronjahmReachHome(event, creature)
    creature:CastSpell(creature, SPELL_SOULSTORM_CHANNEL, true)
end
RegisterCreatureEvent(ENTRY_BRONJAHM, 24, bronjahmReachHome)
