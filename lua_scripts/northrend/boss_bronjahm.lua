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
--   (SelectTarget Random position 1 playerOnly :188; threat-list
--   skip-first + alive-player filter, eck precedent), Talk(SAY_CORRUPT_SOUL 4)
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
-- JustEngagedWith (:98-103) — BossAI::JustEngagedWith base-first:
-- SetBossState(DATA_BRONJAHM 0, IN_PROGRESS), then Talk(SAY_AGGRO 0)
-- (:101) unconditional; RemoveAurasDueToSpell(SPELL_SOULSTORM_CHANNEL
-- 69008) (:102) via the RemoveAura bridge (the arlokk precedent).
-- KilledUnit (:111-115) — Talk(SAY_SLAY 1) player-gated (:114).
-- JustDied (:105-109) — _JustDied() first: SetBossState(DATA_BRONJAHM 0,
-- DONE), then Talk(SAY_DEATH 2) (:108) unconditional. Reset (:83-90) —
-- _Reset() first: SetBossState(DATA_BRONJAHM 0, NOT_STARTED), != DONE-gated
-- (new ev23 leg; the stale "_JustDied passthrough, no bridge" note is
-- retracted — creature:SetBossState is bridged, lua_creature_events.go:5917).
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
--   MoveFollow(me, GetCombatReach(), 0.0f) :134 (PORTED tick 489 —
--   the "no motion bridge" claim was stale) + PURPLE_BANISH_VISUAL
--   self-cast :135 (triggered); summons.Summon has no Lua summon list.
-- GetData(DATA_SOUL_POWER) (:139-154) counts the live corrupted soul
-- fragments in the summon list — PORTED tick 417 as a SetData mirror
-- (JustSummoned +1 / SummonedCreatureDies + SummonedCreatureDespawn
-- -1, GUID-set guarded; see below), read by the Go gate via
-- bossLatchData. Delta: the fragment consume-despawn leg stays out
-- (see below).
-- npc_corrupted_soul_fragment (36535) MovementInform (:241-251) is
--   PORTED as soulFragmentMovementInform (event 28, tick 489):
--   FOLLOW_MOTION_TYPE 14 + DATA_BRONJAHM counter check (the
--   GetGuidDataLow bridge, tick 523) -> triggered CONSUME_SOUL 68861 self-cast +
--   DespawnOrUnsummon; the ev5 spawn latch (bronjahmSpawn) feeds the
--   instance GetGuidData store. Its IsSummonedBy relay (:235-239) needs
--   no registration — the summon funnel already delivers JustSummoned
--   to Bronjahm via ev19. The consume_soul SpellScript redirect
--   (:302-310) stays out (no SpellScript bridge).
-- The four SpellScript/AuraScript hooks (magic_bane mana-power damage
--   recalc :284, consume_soul script-effect redirect :311, soulstorm_visual
--   8-spell periodic ring :345/:350, soulstorm_targeting 10yd exclusion)
--   join the no-SpellScript / no-AuraScript bridge queues.
--
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts are
-- packet-visual), so timers fire unconditionally (the maiden precedent);
-- corrupt-soul/fear picks draw the threat list (GetAITargets) rather than
-- the C++ spell-target machinery, with no target at all when none qualify
-- (Talk + cast stay inside the C++ :188 target-found leg); EVENT_FEAR's
-- { SPELLVALUE_MAX_TARGETS, 1 } custom leg
-- (:201) has no bridge, so the single-target pick (randomFearTarget,
-- threat players alive, tank included) stands in for C++'s self-centered
-- AoE capped at 1 target; phase-2
-- melee keeps swinging (C++ :212 gates DoMeleeAttackIfReady on phase-1;
-- the phase-2 melee stop has no bridge); the phase-1 shadow-bolt melee
-- gate is creature:IsWithinMeleeRange (bridge live since ed086b69);
-- DamageTaken fires pre-application and the C++ arm
-- (boss_bronjahm.cpp:117-118) reads the same pre-application health (Unit.cpp:701
-- runs the hook before the health reduction), so plain GetHealth() is exact;
-- !HealthAbovePct(30) is health <= floor(max*30/100), inclusive of exactly-30.

local ENTRY_BRONJAHM = 36497
local DATA_BRONJAHM = 0 -- forge_of_souls.h:31
local DATA_SOUL_POWER = 1 -- boss_bronjahm.cpp:68

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
local SPELL_CONSUME_SOUL = 68861 -- boss_bronjahm.cpp:42
local FOLLOW_MOTION_TYPE = 14 -- movementGeneratorFollow (creaturemotion.go:4024)
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

-- Achievement write leg (tick 417): C++ GetData(DATA_SOUL_POWER)
-- (boss_bronjahm.cpp:139-154) counts the ALIVE corrupted soul
-- fragments in the boss's summon list at check time. Lua keeps the
-- same count in the boss motion's SetData store, driven by a
-- per-GUID set of live fragment GUIDs: +1 on JustSummoned (ev19),
-- -1 on SummonedCreatureDies (ev21, killed) and
-- SummonedCreatureDespawn (ev20, despawned). The set guards the
-- count against a death+despawn double-fire. The consume leg
-- (npc_corrupted_soul_fragment MovementInform :249 ->
-- SPELL_CONSUME_SOUL + DespawnOrUnsummon) is PORTED tick 489
-- (soulFragmentMovementInform, ev28), so a fragment consumed on
-- arrival despawns -> ev20 -> -1, and the gate no longer over-awards
-- on fights where fragments are left standing.
-- No ev20-only reliance: C++ reaches 0 via _Reset's DespawnAll
-- (ScriptedCreature.cpp:507) -> ev20, but ev20 firing for the
-- _Reset despawn is unverified, and the motion's SetData store
-- would otherwise hold the previous attempt's count until the
-- next fragment event — a zero-fragment kill after a >=4-fragment
-- attempt would read the stale value. The explicit writes below
-- (alive-gated Reset zero + engage zero) are the reliable leg.
-- Declared before initState (malown precedent — a later local is NOT
-- in scope for an earlier-defined closure).
local soulFragments = {}

local function initState(guid)
    cancelTimers(guid)
    phase2[guid] = nil
    soulFragments[guid] = nil
end

local function writeSoulPower(creature)
    local count = 0
    local set = soulFragments[creature:GetGUIDLow()]
    if set then
        for _ in pairs(set) do
            count = count + 1
        end
    end
    creature:SetData(DATA_SOUL_POWER, count)
end

-- C++ :187 SelectTarget(Random, 1, 0.0f, true) — offset 1 pops the
-- current victim (UnitAI.h non-distance list is victim-first), not
-- the threat-sorted head: exclude by victim GUID, then keep alive players.
local function randomPlayer(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local candidates = {}
    local aliveSeen = 0
    for _, t in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = t:GetGUID() == victimGuid
        elseif not t:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and t:GetObjectType() == "Player" and not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Cast-state gate (2026-11-04): own UpdateAI pre-loop (:163) +
-- in-loop (:208) UNIT_STATE_CASTING returns — each event-loop arm
-- below defers 500ms via creature:IsCasting()
-- (lua_creature_events.go:1950) without consuming its re-arm (the
-- mal_ganis idiom); the defer precedes target picks and Talk legs.
-- The DamageTaken phase latch is a separate hook and stays ungated.
-- C++ :170-173 EVENT_MAGIC_BANE: DoCastAOE(SPELL_MAGIC_S_BANE) is
-- self-cast (the DoCastAOE convention); re-arm {8000,20000}ms, both phases.
local function onMagicBane(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "magicbane", 500, function() onMagicBane(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_MAGIC_BANE)
    schedule(guid, "magicbane", {8000, 20000}, function() onMagicBane(creature, guid) end)
end

-- C++ :174-185 EVENT_SHADOW_BOLT: phase-2 is unconditional victim-cast
-- with {1000,2000}ms re-arm (:175-178); phase-1 casts only outside melee
-- range (:181-183, IsWithinMeleeRange gate) with a 2000ms re-arm (:184).
local function onShadowBolt(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadowbolt", 500, function() onShadowBolt(creature, guid) end)
        return
    end
    if phase2[guid] then
        creature:CastSpell(nil, SPELL_SHADOW_BOLT)
        schedule(guid, "shadowbolt", {1000, 2000}, function() onShadowBolt(creature, guid) end)
    else
        local victim = creature:GetVictim()
        if victim and not creature:IsWithinMeleeRange(victim) then
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
    if creature:IsCasting() then
        schedule(guid, "corruptsoul", 500, function() onCorruptSoul(creature, guid) end)
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
    if creature:IsCasting() then
        schedule(guid, "soulstorm", 500, function() onSoulstorm(creature, guid) end)
        return
    end
    creature:Talk(SAY_SOUL_STORM)
    creature:CastSpell(creature, SPELL_SOULSTORM_VISUAL, true)
    creature:CastSpell(creature, SPELL_SOULSTORM)
end

-- C++ :200-203 EVENT_FEAR (PHASE_2 only): CastSpell(nullptr, SPELL_FEAR,
-- { SPELLVALUE_MAX_TARGETS, 1 }) — the nullptr runs the spell's implicit
-- random-enemy selection capped at 1 target, not a victim cast. The Go cast
-- path binds an explicit target GUID only, so nil would always fear the
-- tank; randomFearTarget restores the C++ pick (tank stays in the pool, per
-- the implicit targeting; the existing randomPlayer helper excludes the threat head
-- for the Corrupt Soul SelectTarget(Random, 1) arm and does not
-- apply here). Re-arm {8000,12000}ms.
local function randomFearTarget(creature)
    -- C++ :201 CastSpell(nullptr, FEAR) runs the spell's implicit
    -- random-enemy selection over combat enemies, not instance players:
    -- threat-list players alive, tank included (the old
    -- GetPlayersInWorld pool could fear a player never in combat).
    local candidates = {}
    for _, plr in ipairs(creature:GetAITargets()) do
        if plr:GetObjectType() == "Player" and not plr:IsDead() then
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
    if creature:IsCasting() then
        schedule(guid, "fear", 500, function() onFear(creature, guid) end)
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
    local guid = creature:GetGUIDLow()
    initState(guid)
    -- No fragment can be live at engage, so the count is
    -- definitionally 0 here (the Reset zero below is the wipe leg).
    creature:SetData(DATA_SOUL_POWER, 0)
    -- BossAI::JustEngagedWith (:98-103, base-first) — SetBossState
    -- (DATA_BRONJAHM 0, IN_PROGRESS) via creature:SetBossState
    -- (lua_creature_events.go:5917), then SetCombatPulseDelay(5)
    -- + me-form DoZoneInCombat (ScriptedCreature.cpp:540/542).
    creature:SetBossState(DATA_BRONJAHM, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
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
    local guid = creature:GetGUIDLow()
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

-- C++ :105-109 JustDied: _JustDied() first — SetBossState
-- (DATA_BRONJAHM 0, DONE) via creature:SetBossState — then
-- Talk(SAY_DEATH) (:108) unconditional.
local function bronjahmJustDied(event, creature)
    initState(creature:GetGUIDLow())
    creature:SetBossState(DATA_BRONJAHM, 3)
    creature:Talk(SAY_DEATH)
end

local function bronjahmLeaveCombat(event, creature)
    initState(creature:GetGUIDLow())
end

-- C++ Reset (:83-90): _Reset() first — SetBossState(DATA_BRONJAHM 0,
-- NOT_STARTED) only when the stored state != DONE (the cyanigosa /
-- keleseth precedent: ev23 fires on spawn too, and a DONE boss must
-- stay DONE) — then the phase-1 event seeds (which land on engage in
-- this file, see bronjahmEnterCombat).
local function bronjahmReset(event, creature)
    if creature:GetBossState(DATA_BRONJAHM) ~= 3 then
        creature:SetBossState(DATA_BRONJAHM, 0)
    end
    initState(creature:GetGUIDLow())
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time zero would clobber the fight's count before
    -- the achievement gate reads it; the wipe re-arm is preserved.
    if creature:IsAlive() then
        creature:SetData(DATA_SOUL_POWER, 0)
    end
end

RegisterCreatureEvent(ENTRY_BRONJAHM, 1, bronjahmEnterCombat)
RegisterCreatureEvent(ENTRY_BRONJAHM, 2, bronjahmLeaveCombat)
RegisterCreatureEvent(ENTRY_BRONJAHM, 3, bronjahmKilledUnit)
RegisterCreatureEvent(ENTRY_BRONJAHM, 4, bronjahmJustDied)
RegisterCreatureEvent(ENTRY_BRONJAHM, 9, bronjahmDamageTaken)
RegisterCreatureEvent(ENTRY_BRONJAHM, 23, bronjahmReset)

-- C++ instance_forge_of_souls::OnCreatureCreate (instance_forge_of_souls.cpp:54-76)
-- latches the boss GUID under DATA_BRONJAHM for GetGuidData consumers.
-- The Lua mirror is creature:SetGuidData from the ev5 spawn handler
-- (illidari-council precedent); motionFor fires ev5 on first spawn and
-- on respawn. The fragment's MovementInform leg below reads it.
local function bronjahmSpawn(event, creature)
    creature:SetGuidData(DATA_BRONJAHM)
end
RegisterCreatureEvent(ENTRY_BRONJAHM, 5, bronjahmSpawn)

-- C++ boss_bronjahmAI::JustSummoned (boss_bronjahm.cpp:128-136):
-- entry NPC_CORRUPTED_SOUL_FRAGMENT 36535 -> summons.Summon (no Lua
-- summon list) + SetReactState(REACT_PASSIVE) + MoveFollow(me,
-- GetCombatReach(), 0.0f) + triggered self-cast PURPLE_BANISH_VISUAL
-- 68862. MoveFollow PORTED (tick 489 — the "no motion bridge" claim
-- was stale; creature:MoveFollow is bridged, lua_creature_events.go:5097,
-- boss_midnight.lua precedent). The fragment IsSummonedBy
-- instance-GetGuidData leg stays out. Event 19 fires on the summoner
-- for spell-effect summons through the shared summonTempCreature funnel
-- (boss_ai.go:1734), so the ported CORRUPT_SOUL cast delivers this arm.
local function bronjahmJustSummoned(event, creature, summon)
    if summon:GetEntry() ~= ENTRY_CORRUPTED_SOUL_FRAGMENT then
        return
    end
    local guid = creature:GetGUIDLow()
    local set = soulFragments[guid]
    if not set then
        set = {}
        soulFragments[guid] = set
    end
    set[summon:GetGUIDLow()] = true
    writeSoulPower(creature)
    summon:SetReactState(REACT_PASSIVE)
    summon:MoveFollow(creature, creature:GetCombatReach(), 0)
    summon:CastSpell(summon, SPELL_PURPLE_BANISH_VISUAL, true)
end
RegisterCreatureEvent(ENTRY_BRONJAHM, 19, bronjahmJustSummoned)

-- C++ GetData(DATA_SOUL_POWER) counts live fragments, so a fragment
-- leaving the summon list drops the count: killed
-- (SummonedCreatureDies, ev21, fireCreatureDied summoner-OwnerGUID
-- link) or despawned (SummonedCreatureDespawn, ev20). The GUID-set
-- guard makes a death+despawn double-fire a no-op.
local function bronjahmSummonDied(event, creature, summon, killer)
    if summon:GetEntry() ~= ENTRY_CORRUPTED_SOUL_FRAGMENT then
        return
    end
    local set = soulFragments[creature:GetGUIDLow()]
    if set then
        set[summon:GetGUIDLow()] = nil
    end
    writeSoulPower(creature)
end

local function bronjahmSummonDespawned(event, creature, summon)
    if summon:GetEntry() ~= ENTRY_CORRUPTED_SOUL_FRAGMENT then
        return
    end
    local set = soulFragments[creature:GetGUIDLow()]
    if set then
        set[summon:GetGUIDLow()] = nil
    end
    writeSoulPower(creature)
end
RegisterCreatureEvent(ENTRY_BRONJAHM, 21, bronjahmSummonDied)
RegisterCreatureEvent(ENTRY_BRONJAHM, 20, bronjahmSummonDespawned)

-- C++ npc_corrupted_soul_fragmentAI::MovementInform (boss_bronjahm.cpp:241-251):
-- FOLLOW_MOTION_TYPE only, and only when the followed target is Bronjahm —
-- the instance DATA_BRONJAHM object GUID's counter (GetCounter() = low 32
-- bits) must equal the inform id. Go passes uint32(targetGUID) as the id
-- (creaturemotion.go:4406-4410), so the Lua mirror compares via
-- GetGuidDataLow(DATA_BRONJAHM) — the low-32-bits numeric bridge
-- (lua_creature_events.go); a GetGuidData(...) % 2^32 cannot work
-- (UInt64 userdata has no __mod, tick 523). On match: triggered
-- self-cast CONSUME_SOUL 68861 (the C++ CastSpell(nullptr, ...) leg, per
-- the spirit-shade self-cast precedent) + immediate despawn
-- (DespawnOrUnsummon() with no args = ms 0, C++-exact). The fragment's
-- IsSummonedBy leg (:235-239, relaying JustSummoned to Bronjahm) needs
-- no registration: the summon funnel already delivers JustSummoned to
-- Bronjahm via ev19 (see bronjahmJustSummoned above), and the Lua
-- GUID-set absorbs C++'s double delivery. The consume_soul SpellScript
-- itself (script-effect redirect :302-310) stays out — no SpellScript
-- bridge — so the cast fires its DBC effect only. Dormant until the
-- DATA_BRONJAHM latch lands (ev5 spawn, above); an unlatched store
-- reads 0 and the id check fails closed.
local function soulFragmentMovementInform(event, creature, type, id)
    if type ~= FOLLOW_MOTION_TYPE then
        return
    end
    if id ~= creature:GetGuidDataLow(DATA_BRONJAHM) then
        return
    end
    creature:CastSpell(creature, SPELL_CONSUME_SOUL, true)
    creature:DespawnOrUnsummon()
end
RegisterCreatureEvent(ENTRY_CORRUPTED_SOUL_FRAGMENT, 28, soulFragmentMovementInform)

-- C++ boss_bronjahmAI::JustReachedHome (:94-96): _JustReachedHome() is
-- instance bookkeeping (no bridge); DoCast(me, SPELL_SOULSTORM_CHANNEL,
-- true) is a TRIGGERED self-cast, ported via the Eluna third-arg
-- triggered convention. Event 24 fires after the home-move completes.
local function bronjahmReachHome(event, creature)
    creature:CastSpell(creature, SPELL_SOULSTORM_CHANNEL, true)
end
RegisterCreatureEvent(ENTRY_BRONJAHM, 24, bronjahmReachHome)
