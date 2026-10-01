-- Boss Headless Horseman (Scarlet Monastery)
-- Lua port of src/server/scripts/EasternKingdoms/ScarletMonastery/
-- boss_headless_horseman.cpp
-- (struct boss_headless_horseman : public ScriptedAI, registered by
-- AddSC_boss_headless_horseman in eastern_kingdoms_script_loader.cpp
-- (line 279) via RegisterScarletMonasteryCreatureAI ->
-- RegisterCreatureAIWithFactory (scarlet_monastery.h:100)).
-- Entry (verifiable from the C++ sources): NPC_HEADLESS_HORSEMAN =
-- 23682 in the SMCreatureIds enum in ScarletMonastery/
-- scarlet_monastery.h (lines 69-75, same-block header enum); the same
-- enum also names NPC_HEADLESS_HORSEMAN_HEAD = 23775 /
-- NPC_PULSING_PUMPKIN = 23694 / NPC_PUMPKIN_FIEND = 23545 /
-- NPC_FLAME_BUNNY = 23686 / NPC_EARTH_BUNNY = 23758 /
-- NPC_SIR_THOMAS = 23904, and SMGameObjectIds names
-- GO_PUMPKIN_SHRINE = 186267 / GO_LOOSELY_TURNED_SOIL = 186314.
-- Whole-server-tree grep confirms boss_headless_horseman.cpp as the
-- only source of "boss_headless_horseman" (struct line 340,
-- AddSC_ line 1051).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Verifiable numbers (file's own enums): SPELL_HEADLESS_HORSEMAN_CLEAVE =
-- 42587 / SPELL_HEADLESS_HORSEMAN_BURNING_COSMETIC = 42971
-- (HeadlessHorsemanSpells); SAY_HORSEMAN_ENTRANCE = 0 / SAY_REJOINED = 1 /
-- SAY_CONFLAGRATION = 2 / SAY_SPROUTING_PUMPKINS = 3 / SAY_DEATH = 4 /
-- SAY_KILL_PLAYER = 5 (HeadlessHorsemanSays, Boss leg); EVENT_HORSEMAN_CLEAVE
-- = 1 (HeadlessEvents); Cleave 13s init -> Repeat(6s, 12s) DoCastVictim;
-- phase-2 cleave re-arm 16s, phase-3 re-arm 9s.
-- Ported arms:
-- - JustEngagedWith's EVENT_HORSEMAN_CLEAVE -> CreateLuaEvent timer armed
--   on OnEnterCombat (1), cancelled on 2/4/23 (maiden convention), with
--   DoCastVictim -> GetVictim + CastSpell (mr_smite convention),
--   C++-exact 13s init / Repeat(6s, 12s).
-- - KilledUnit's Talk(SAY_KILL_PLAYER) with the TYPEID_PLAYER gate ->
--   OnTargetDied (3) with a victim:IsPlayer() gate (illidan convention).
-- - JustDied's Talk(SAY_DEATH) + DoCastSelf(SPELL_HEADLESS_HORSEMAN_
--   BURNING_COSMETIC) -> OnDied (4) (maiden precedent: cancel timers,
--   then the death arm); the cosmetic cast is packet-visual like all
--   creature self-casts on this surface.
-- Deviations from C++: C++ re-arms cleave phase-pinned (16s in PHASE_2,
-- 9s in PHASE_3) and drives the whole headless-phase machine from
-- DamageTaken's cheat-death latches (damage >= health -> damage = 0 +
-- StartPhase); that phase machine (head reposition/send/return spells,
-- cross-creature DoActions, MovementInform pathing) has no bridges, so
-- the port runs the phase-1 cleave cadence flat. JustDied's
-- _instance->SetData(DATA_HORSEMAN_EVENT_STATE, DONE) leg is blocked on
-- the instance-script model; the SummonCreature(NPC_SIR_THOMAS) leg has
-- no summon bridge (dark-rider precedent); the LFG FinishDungeon leg has
-- no LFG bridge. JustEngagedWith's SetCombatPulseDelay/setActive legs
-- have no bridges; its EVENT_RANDOM_LAUGH arm has no sound bridge
-- (DoPlaySoundToSet — gurtogg precedent, boss_ai.go:834); its
-- head->AI()->DoZoneInCombat() leg hangs off the instance-script model.
-- Unmodeled (documented-only — no bridges on the Lua surface):
-- - DamageTaken's phase-transition latch (damage negation + StartPhase's
--   immune aura / REACT_PASSIVE / AttackStop / EVENT_START_NEXT_
--   HEADLESS_PHASE -> DoCastAOE(42410) + DoCast(head, 42399) + confuse/
--   transform auras): porting the damage negation alone, with no head
--   phase machine to hand off to, would make the boss unkillable — the
--   whole arm strands on the unbridged instance-script head/phase model.
-- - DoAction's ACTION_HORSEMAN_EVENT_START (flight-path intro via
--   MoveSmoothPath + yell-timer/laugh auras), ACTION_HORSEMAN_REQUEST_
--   BODY (MovePoint(POINT_HEAD) chase), ACTION_HEAD_RETURN_TO_BODY
--   (phase-delayed re-arm + Talk(SAY_REJOINED)), ACTION_HEAD_IS_DEAD:
--   no MotionMaster bridge (selin_fireheart precedent), no MovementInform
--   bridge (kalecgos precedent), and every head lookup goes through
--   _instance->GetCreature(DATA_HORSEMAN_HEAD) (instance-script model
--   blocked).
-- - EVENT_CONFLAGRATE (phase 2): SelectTarget(SelectTargetMethod::
--   Random, 0, 30.0f, true, false) — NO RANDOM-TARGET BRIDGE (delrissa
--   precedent); phase-pinned on the unmodeled phase machine.
-- - EVENT_SUMMON_PUMPKIN (phase 3): phase-pinned; the 52236 aura's whole
--   effect is the pumpkin-spawn machinery (no summon bridge).
-- - Reset/JustReachedHome/InitializeAI legs: SetDisableGravity/SetHover
--   (no bridge), _summons.DespawnAll (no despawn bridge), SetReactState
--   (no react-state bridge — koltira precedent), SetImmuneToPC (no
--   bridge), HandleInitialSetup's DoCastSelf(42413/43877) self-casts are
--   stranded halves (rageclaw/phoenix_tk precedent: C++ Reset/JustAppeared
--   run at spawn Initialize, Lua OnReset (23) fires only on reset/evade).
-- - npc_headless_horseman_head (23775): every arm hangs off SpellScript/
--   AuraScript effects (42399 send-head, 42603 HP-check AuraScript —
--   AuraScript check handlers not modeled, standing blocker), DoAction
--   legs (ACTION_HEAD_START_HEAD_PHASE/H P_67/HP_34), and
--   _instance->GetCreature(DATA_HEADLESS_HORSEMAN); its UpdateAI is
--   otherwise a PassiveAI event pump. Nothing genuinely bridgeable —
--   not registered (an empty artifact would be a stub).
-- - npc_pulsing_pumpkin (23694): Reset self-casts + the sprout arm fires
--   only via DoAction(ACTION_PUMPKIN_SPROUTING_FINISHED) from the
--   unmodeled 42281 SpellScript; UpdateEntry + DoZoneInCombat have no
--   bridges. Documented-only.
-- - npc_flame_bunny (23686): Reset's cosmetic self-cast growth schedule
--   (43184 -> 43148 x3) — stranded-half Reset self-casts (rageclaw/
--   phoenix_tk precedent); no Talk, no combat. Documented-only.
-- - npc_sir_thomas (23904): Reset self-casts + a SpellHit(42818) handler
--   — SpellHit handlers not modeled (draenei_survivor precedent).
--   Documented-only.
-- - go_loosely_turned_soil (186314) / go_headless_horseman_pumpkin
--   (186267): OnGossipHello/OnQuestReward/OnGossipSelect all hang off
--   _instance->GetData/SetData (DATA_HORSEMAN_EVENT_STATE/DATA_START_
--   HORSEMAN_EVENT — instance-script model blocked) plus the gossip and
--   quest-reward arms, which have no bridges on this surface.
--   Documented-only.

local SPELL_HORSEMAN_CLEAVE = 42587
local SPELL_BURNING_COSMETIC = 42971

local SAY_DEATH = 4
local SAY_KILL_PLAYER = 5

local ENTRY_HEADLESS_HORSEMAN = 23682

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_HORSEMAN_CLEAVE)
    end
    schedule(guid, "cleave", math.random(6000, 12000), function() onCleave(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    schedule(guid, "cleave", 13000, function() onCleave(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onTargetDied(_, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_KILL_PLAYER)
    end
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_BURNING_COSMETIC)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 4, onDied)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 23, onCombatEnd)
