-- Temple of Ahn'Qiraj: Viscidus — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_viscidus.cpp
-- (boss_viscidusAI : public BossAI(creature, DATA_VISCIDUS);
-- GetAI via GetAQ40AI<boss_viscidusAI>(creature)
-- (instance_temple_of_ahnqiraj gate); AddSC_boss_viscidus at end
-- registers boss_viscidus + npc_glob_of_viscidus;
-- kalimdor_script_loader.cpp decl 88 / call after cthun — second
-- "// Temple of ahn'qiraj" loader group).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "boss_viscidus" (loader decl/call lines only otherwise).
-- Entry: temple_of_ahnqiraj.h:68 names NPC_VISCIDUS = 15299;
-- NPC_GLOB_OF_VISCIDUS = 15667 (:69); DATA_VISCIDUS = 7 (:37). The
-- creature_template ScriptName bindings stay DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. (Header correction 2026-11-14, round116:
-- ev9/ev14 are NOT registered — the DamageTaken/SpellHit arms stay
-- out, see below.) Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Engage (JustEngagedWith -> InitSpells): self-cast TOXIN 26575;
--   schedule POISONBOLT_VOLLEY 25991 (10-15s init -> 10-15s) and
--   POISON_SHOCK 25993 (7-12s init -> 7-12s).
-- Unmodeled (documented-only, no bridges):
-- - The whole frost/slow/freeze machine (SpellHit event 14):
--   STAY OUT, re-verified 2026-11-13 against the CURRENT runtime.
--   C++ (boss_viscidus.cpp:154-181): gate is
--   (spellInfo->GetSchoolMask() & SPELL_SCHOOL_MASK_FROST) &&
--   _phase == PHASE_FROST (1) && GetHealthPct() > 5.0; ++_hitcounter
--   (uint8, ctor/Reset Initialize() :109-116, 0/FROST); >=100
--   HITCOUNTER_SLOW: Talk(EMOTE_SLOW 0) + DoCast SLOWED 26034;
--   >=150 HITCOUNTER_SLOW_MORE: Talk(EMOTE_FREEZE 1) +
--   RemoveAura(SLOWED) + DoCast SLOWED_MORE 26036; >=200
--   HITCOUNTER_FREEZE: counter = 0, Talk(EMOTE_FROZEN 2),
--   _phase = PHASE_MELEE (2), DoCast FREEZE 25937,
--   RemoveAura(SLOWED_MORE), ScheduleEvent(EVENT_RESET_PHASE, 15s)
--   (UpdateAI :243-246 resets counter/phase to FROST). Thresholds
--   are >= with an else-if chain, so the SLOW/SLOW_MORE legs re-fire
--   on EVERY hit in range (no once-guard) — a port would replicate
--   that, not a one-shot.
--   Blocker: ev14 itself IS bridged (runtime.go:379
--   CreatureEventOnHitBySpell = 14; fire sites boss_ai.go:4267-4282
--   creature-caster, spells.go:19914/30192 player-caster,
--   creature_area_targets.go:2667, pet_combat.go:8073/8486/9232),
--   args (event, creature, caster, spellId) — spellId ONLY, no
--   spell object and no school mask. GetSchoolMask exists solely on
--   the aura ProcEventInfo (lua_aura.go:1267; runtime.go:516-517),
--   unreachable from a creature ev14 handler, and there is no
--   spell-info-by-ID lookup in Lua (globals.go GetSpellId is
--   item-template only); spells.go:681 spellSchoolMaskFrost = 16 is
--   Go-internal. Porting by spellId enumeration would misfire on
--   non-frost spells and miss unlisted frost ones — the C++ gate is
--   any frost-school spell, an unbounded set. In-tree ev14 ports
--   (ossirian, veklor, zuljin vortex) all gate on one exact spellId;
--   no school-gated precedent exists.
--   Everything ELSE in the arm is bridged and would port cleanly if
--   the gate ever lands: GetHealthPct (object.go:209 fallback,
--   lua_objects.go:338), Talk(textId) (lua_creature_events.go:1252),
--   RemoveAura (:1748), CastSpell (used elsewhere in this file),
--   15s reset via the existing CreateLuaEvent schedule() helper.
--   Lua currently models no _phase/_hitcounter state at all, so
--   there is nothing partial to wire — and the freeze leg's whole
--   purpose is entering PHASE_MELEE, which the ev9 DamageTaken arm
--   (also unported) depends on. No dormant port: a spellId-gated
--   approximation would enter MELEE on wrong hits.
-- - DamageTaken (event 9): C++ only acts in PHASE_MELEE, which is
--   unreachable in Lua while the SpellHit freeze leg is absent, so
--   the hitcounter (CRACK 50 + Talk 3, SHATTER 100 + Talk 4,
--   EXPLODE 150 + Talk 5, self-cast VISCIDUS_EXPLODE 25938,
--   SetVisible(false), RemoveAura(TOXIN/FREEZE)) is dead code here
--   — documented, not ported. (HasUnitState(MELEE_ATTACKING) IS
--   bridged — lua_creature_events.go:2489, state 0x2,
--   lua_unit_state.go:182 — and creature:SummonCreature is live
--   (onyxia whelp/lair-guard legs); the blocker is the PHASE_MELEE
--   gate itself, chained on the row-43 frost-school SpellHit gate.
--   Corrected 2026-11-13, round-14 row 44.)
-- - The glob explosion / rejoin machinery: NPC_GLOB_OF_VISCIDUS
--   15667 spawns on a 40yd ring around ViscidusCoord
--   (-7992.36, 908.19, -52.62) — the creature:SummonCreature bridge
--   is live (stale "no bridge" retracted), but the EXPLODE arm sits
--   behind the unported PHASE_MELEE machine (frost spell-school
--   SpellHit gate has no bridge), so the spawn never fires here;
--   MovePoint to ROOM_CENTER, MovementInform -> REJOIN_VISCIDUS
--   25896 + UnSummon (no-motion bridge); the glob JustDied leg shrinks
--   Viscidus (SetHealth - maxhealth/20 + SHRINKS 25893) or kills
--   him under 5% hp — all cross-AI/instance legs, no bridge. Per
--   the no-placeholder-Lua rule, no npc_glob_of_viscidus.lua file
--   is created (cthun tentacle precedent needs real content).
-- - JustDied: the VISCIDUS_SUICIDE 26003 self-cast is ported (event
--   4); summons.DespawnAll() stays out (no bridge); _JustDied()
--   PORTED as the ev4 DONE leg (creature:SetBossState,
--   lua_creature_events.go:5917).
-- - EnterEvadeMode: summons.DespawnAll() + instance leg.
-- - BossAI ctor leg DATA_VISCIDUS and _Reset() PORTED as the ev1
--   IN_PROGRESS + ev23 NOT_STARTED legs (see onEnterCombat / onReset).
local DATA_VISCIDUS = 7 -- temple_of_ahnqiraj.h:37
local ENTRY = 15299

local SPELL_POISON_SHOCK      = 25993
local SPELL_POISONBOLT_VOLLEY = 25991
local SPELL_TOXIN             = 26575
local SPELL_VISCIDUS_SUICIDE  = 26003

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

-- C++ InitSpells leg: self-cast TOXIN, arm volley 10-15s,
-- shock 7-12s. Volley hits the whole room in C++ (DoCast on self);
-- no AoE bridge — self-cast convention like the other AQ ports.
local function onVolley(creature, guid)
    creature:CastSpell(creature, SPELL_POISONBOLT_VOLLEY)
    schedule(guid, "volley", math.random(10000, 15000), function()
        onVolley(creature, guid)
    end)
end

-- C++ EVENT_POISON_SHOCK: DoCast(me, 25993), init 7-12s -> 7-12s.
local function onShock(creature, guid)
    creature:CastSpell(creature, SPELL_POISON_SHOCK)
    schedule(guid, "shock", math.random(7000, 12000), function()
        onShock(creature, guid)
    end)
end

-- C++ JustEngagedWith: BossAI leg (no instance bridge), events
-- reset, InitSpells: self-cast TOXIN + schedule both timers.
local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ JustEngagedWith calls BossAI::JustEngagedWith first
    -- (boss_viscidus.cpp:183-186) — SetBossState(DATA_VISCIDUS, IN_PROGRESS).
    creature:SetBossState(DATA_VISCIDUS, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:CastSpell(creature, SPELL_TOXIN)
    schedule(guid, "volley", math.random(10000, 15000), function()
        onVolley(creature, guid)
    end)
    schedule(guid, "shock", math.random(7000, 12000), function()
        onShock(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset _Reset() leg (boss_viscidus.cpp:109-111): alive-gated and
    -- != DONE-gated (ScriptedCreature.cpp:499-511; moroes/algalon precedent).
    if creature:IsAlive() and creature:GetBossState(DATA_VISCIDUS) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_VISCIDUS, BOSS_STATE_NOT_STARTED)
    end
end

-- C++ boss_viscidusAI::JustDied (:203-208): non-triggered self-cast
-- SPELL_VISCIDUS_SUICIDE (2-arg DoCast(me, ...)); summons.DespawnAll()
-- stays out (no bridge); _JustDied() PORTED as the ev4 DONE leg
-- (C++ order: cast, DespawnAll, _JustDied).
local function onDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_VISCIDUS_SUICIDE)
    creature:SetBossState(DATA_VISCIDUS, BOSS_STATE_DONE)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
