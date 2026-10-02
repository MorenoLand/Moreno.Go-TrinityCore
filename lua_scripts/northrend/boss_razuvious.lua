-- Razuvious (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_razuvious.cpp
-- (boss_razuvious (BossAI, BOSS_RAZUVIOUS); npc_dk_understudy
-- (ScriptedAI); AddSC_boss_razuvious registers both; loader decl 63 /
-- call 258 per northrend_script_loader.cpp — the FIFTH Naxxramas group
-- in AddNorthrendScripts(), immediately after AddSC_boss_grobbulus(),
-- under the "// Naxxramas" marker; the call after it is
-- AddSC_boss_kelthuzad()).
-- Entry: 16061 Razuvious (naxxramas.h NPC_RAZUVIOUS — kalecgos pass;
-- instance_naxxramas.cpp binds NPC_RAZUVIOUS -> RazuviousGUID and
-- DATA_RAZUVIOUS -> RazuviousGUID; the CreatureScript ScriptName
-- binding is instance-shimmed, the creature_template binding DB-side
-- as usual). npc_dk_understudy: NPC_DK_UNDERSTUDY = 16803 (same pass).
-- Sole-source verified: whole-server-tree grep for "boss_razuvious"
-- hits boss_razuvious.cpp only (loader carries only the decl/call
-- lines); grep for "npc_dk_understudy" hits the .cpp only; zero sql/
-- hits. No razuvious lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent). C++ randtime -> math.random
-- (amanitar / boss_black_knight precedent). Victim gating: C++
-- KilledUnit checks TYPEID_PLAYER or (TYPEID_UNIT and entry
-- NPC_DK_UNDERSTUDY); Lua expresses it via victim:GetObjectType()
-- (nalorakk precedent) plus victim:GetEntry() for creatures.
-- Ported arms (C++-exact for all modeled arms):
-- boss_razuvious: JustEngagedWith Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith instance-bookkeeping leg, me->StopMoving,
-- summons.DoZoneInCombat have no bridge — tharon_ja precedent);
-- KilledUnit Talk(SAY_SLAY 1) (event 3 — C++-GATED: Talk fires only
-- when the victim is a player or a TYPEID_UNIT with entry
-- NPC_DK_UNDERSTUDY 16803 — the nalorakk player-gate variant, extended
-- with the understudy-entry arm); EVENT_STRIKE —
-- DoCastVictim(Unbalancing Strike 26613), 21s init, 6s repeat
-- (moroes precedent); JustDied Talk(SAY_DEATH 3) (event 4; the
-- RemoveCharmAuras / events.Reset / instance->SetBossState bookkeeping
-- has no bridge — tharon_ja precedent). All timers cancelled on
-- 2/4/23 (gargolmar precedent). No summon-arm Talk orphaning:
-- EVENT_KNIFE's target leg rides the unbridged SelectTarget.
-- Unmodeled (no bridges — documented, not wired):
-- boss_razuvious SpellHit — SPELL_UNDERSTUDY_TAUNT 29060 ->
-- Talk(SAY_TAUNTED 2, caster) — SpellHit never fires in the Lua
-- surface (SpellHit ruling); JustDied — DoCastAOE(Hopeless 29125,
-- triggered) — no DoCastAOE bridge (terestian/shazzrah precedent) +
-- the summons RemoveCharmAuras leg — summon STRAND absent;
-- JustEngagedWith EVENT_ATTACK (7s) — SetCombatMovement(true) +
-- MoveChase — no motion bridges; EVENT_SHOUT (16s init, 16s repeat) —
-- DoCastAOE(Disrupting Shout 29107) — no DoCastAOE bridge;
-- EVENT_KNIFE (10s init, randtime(10s,15s) repeat) —
-- SelectTarget(Random, 0, 45.0f) -> DoCast(Jagged Knife 55550) — no
-- random-target SelectTarget bridge (cairne/kazzak precedent);
-- Reset — SetCombatMovement(false) + _Reset() — no bridges;
-- InitializeAI / JustReachedHome — SummonAdds: SummonCreatureGroup(
-- SUMMON_GROUP_10MAN 1) + 25-man SUMMON_GROUP_25MAN 2 — no summon
-- bridge + no difficulty bridge (kelidan precedent).
-- npc_dk_understudy (ScriptedAI, 16803 — joins the
-- entry-verifiable-but-bridge-blocked queue; ScriptedAI has no
-- RegisterLuaBoss-style surface): LoadEquipment(1) — no equipment
-- bridge; JustEngagedWith — SetUInt32Value(UNIT_NPC_EMOTESTATE,
-- EMOTE_ONESHOT_NONE) + cross-AI DoZoneInCombat on razuvious via
-- _instance->GetGuidData(DATA_RAZUVIOUS) — no instance / cross-AI
-- bridges; JustReachedHome — DespawnOrUnsummon when BOSS_RAZUVIOUS ==
-- DONE — instance model; UpdateAI — bloodStrikeTimer DoCastVictim(
-- SPELL_UNDERSTUDY_BLOOD_STRIKE 61696) — port-pattern-ready in
-- isolation (moroes precedent) but gated on !me->isPossessedByPlayer()
-- — no charm-possession bridge (jedoga over-cast bar); OnCharmed —
-- no bridge.

local ENTRY_RAZUVIOUS = 16061
local ENTRY_DK_UNDERSTUDY = 16803

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 3

local SPELL_UNBALANCING_STRIKE = 26613

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

-- C++ EVENT_STRIKE: DoCastVictim(Unbalancing Strike 26613), 21s init,
-- 6s repeat.
local function strikeTick(creature, guid)
    creature:CastSpell(nil, SPELL_UNBALANCING_STRIKE)
    schedule(guid, "strike", 6000, function()
        strikeTick(creature, guid)
    end)
end

local function razyviousEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "strike", 21000, function()
        strikeTick(creature, guid)
    end)
end

local function razyviousLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_SLAY) when victim->GetTypeId() ==
-- TYPEID_PLAYER or (TYPEID_UNIT and GetEntry() == NPC_DK_UNDERSTUDY
-- 16803).
local function razyviousTargetDied(event, creature, victim)
    if victim then
        local otype = victim:GetObjectType()
        if otype == "Player" or (otype == "Creature" and victim:GetEntry() == ENTRY_DK_UNDERSTUDY) then
            creature:Talk(SAY_SLAY)
        end
    end
end

local function razyviousDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

local function razyviousReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_RAZUVIOUS, 1, razyviousEnterCombat)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 2, razyviousLeaveCombat)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 3, razyviousTargetDied)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 4, razyviousDied)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 23, razyviousReset)
