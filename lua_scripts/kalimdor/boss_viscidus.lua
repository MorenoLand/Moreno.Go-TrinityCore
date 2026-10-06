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
-- OnDied, 9 OnDamageTaken, 14 OnSpellHit, 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Engage (JustEngagedWith -> InitSpells): self-cast TOXIN 26575;
--   schedule POISONBOLT_VOLLEY 25991 (10-15s init -> 10-15s) and
--   POISON_SHOCK 25993 (7-12s init -> 7-12s).
-- Unmodeled (documented-only, no bridges):
-- - The whole frost/slow/freeze machine (SpellHit event 14): C++
--   counts hits whose GetSchoolMask() has SPELL_SCHOOL_MASK_FROST
--   while phase is FROST and health > 5%, applying SLOWED 26034
--   + Talk(EMOTE_SLOW 0) at 100 hits, SLOWED_MORE 26036 +
--   Talk(EMOTE_FREEZE 1) at 150, FREEZE 25937 + Talk(EMOTE_FROZEN
--   2) at 200 (phase -> MELEE, 15s EVENT_RESET_PHASE back to
--   FROST). No spell-school bridge exists in Lua — porting by
--   spellId would misfire on every school, so this leg stays out.
-- - DamageTaken (event 9): C++ only acts in PHASE_MELEE, which is
--   unreachable in Lua while the SpellHit freeze leg is absent, so
--   the hitcounter (CRACK 50 + Talk 3, SHATTER 100 + Talk 4,
--   EXPLODE 150 + Talk 5, self-cast VISCIDUS_EXPLODE 25938,
--   SetVisible(false), RemoveAura(TOXIN/FREEZE)) is dead code here
--   — documented, not ported. (The EXPLODE branch also needs
--   HasUnitState(UNIT_STATE_MELEE_ATTACKING) — no casting-state
--   bridge — plus SummonCreature for the globe ring, which has
--   zero Lua usage.)
-- - The glob explosion / rejoin machinery: NPC_GLOB_OF_VISCIDUS
--   15667 spawns (SummonCreature — no bridge) on a 40yd ring
--   around ViscidusCoord (-7992.36, 908.19, -52.62), MovePoint
--   to ROOM_CENTER then MovementInform -> REJOIN_VISCIDUS 25896
--   + UnSummon (no-motion bridge); the glob JustDied leg shrinks
--   Viscidus (SetHealth - maxhealth/20 + SHRINKS 25893) or kills
--   him under 5% hp — all cross-AI/instance legs, no bridge. Per
--   the no-placeholder-Lua rule, no npc_glob_of_viscidus.lua file
--   is created (cthun tentacle precedent needs real content).
-- - JustDied: self-cast VISCIDUS_SUICIDE 26003 + _JustDied()
--   instance legs — no bridge (no OnDied self-cast precedent).
-- - EnterEvadeMode: summons.DespawnAll() + instance leg.
-- - BossAI ctor leg DATA_VISCIDUS and _Reset() — instance bridge,
--   standing.
local ENTRY = 15299

local SPELL_POISON_SHOCK      = 25993
local SPELL_POISONBOLT_VOLLEY = 25991
local SPELL_TOXIN             = 26575

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
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_TOXIN)
    schedule(guid, "volley", math.random(10000, 15000), function()
        onVolley(creature, guid)
    end)
    schedule(guid, "shock", math.random(7000, 12000), function()
        onShock(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
