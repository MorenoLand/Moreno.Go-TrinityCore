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
-- OnDied, 9 OnDamageTaken, 14 OnHitBySpell, 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Engage (JustEngagedWith -> InitSpells): self-cast TOXIN 26575;
--   schedule POISONBOLT_VOLLEY 25991 (10-15s init -> 10-15s) and
--   POISON_SHOCK 25993 (7-12s init -> 7-12s).
-- Unmodeled (documented-only, no bridges):
-- - The frost/slow/freeze machine (SpellHit event 14): PORTED
--   2026-11-15 on the new creature:GetSpellSchoolMask(spellId)
--   bridge (lua_creature_events.go, the GetSpellFamilyFlags twin:
--   Spell.dbc SchoolMask, 0 for an unknown spell). C++
--   (boss_viscidus.cpp:154-181): gate is
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
--   on EVERY hit in range (no once-guard) — the port replicates
--   that, not a one-shot. The 15s reset rides the existing
--   CreateLuaEvent schedule() helper ("resetPhase" key, cancelled
--   with the rest on evade/reset/death). The SLOWED/SLOWED_MORE/
--   FREEZE self-casts are dormant until their Spell.dbc rows land
--   (25937/26034/26036 unseeded — the 25991/25993/26575 precedent);
--   the Talks, phase flip and counter are live for any seeded
--   frost-school spell (31266 Blizzard, 47528 Mind Freeze). Player
--   frost ranks (Frostbolt/Frost Nova/Cone of Cold/Blizzard) need
--   the wago-verified seed batch — the gate reads 0 for unseeded
--   ids, so no misfire, just no counter advance until they land.
-- - DamageTaken (event 9): PORTED 2026-11-15 — C++ only acts in
--   PHASE_MELEE (:115-152): ++_hitcounter (the SAME counter the
--   freeze leg zeroed, so CRACK/SHATTER count melee-phase hits
--   from 0); ==50 HITCOUNTER_CRACK Talk(EMOTE_CRACK 3), ==100
--   HITCOUNTER_SHATTER Talk(EMOTE_SHATTER 4). No postDamage guard:
--   this is a plain DamageTaken arm, not a HealthBelowPct arm —
--   C++ fires it on the killing blow too, and Go fires ev9 before
--   damage is applied (lua_creature_events.go fireCreatureDamageTaken,
--   Unit::DealDamage :697-702). The EXPLODE leg (>=150 + attacker
--   HasUnitState(MELEE_ATTACKING)) stays OUT: it flips to PHASE_GLOB,
--   casts 25938, hides the boss and spawns the glob ring — the glob
--   MovePoint/rejoin + summons-empty return legs have no bridge
--   (see below), so porting the flip would strand Viscidus
--   invisible; at >=150 hits nothing fires and the boss stays
--   killable in MELEE.
-- - The glob explosion / rejoin machinery: NPC_GLOB_OF_VISCIDUS
--   15667 spawns on a 40yd ring around ViscidusCoord
--   (-7992.36, 908.19, -52.62) — the creature:SummonCreature bridge
--   is live (stale "no bridge" retracted), and PHASE_MELEE is now
--   reachable, but the EXPLODE arm stays out (see above): the glob
--   MovePoint to ROOM_CENTER, MovementInform -> REJOIN_VISCIDUS
--   25896 + UnSummon (no-motion bridge), and the UpdateAI
--   PHASE_GLOB && summons.empty() return leg (no per-tick UpdateAI
--   hook) have no bridge; the glob JustDied leg shrinks Viscidus
--   (SetHealth - maxhealth/20 + SHRINKS 25893) or kills him under
--   5% hp — all cross-AI/instance legs, no bridge. Per the
--   no-placeholder-Lua rule, no npc_glob_of_viscidus.lua file
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
local SPELL_VISCIDUS_SLOWED      = 26034
local SPELL_VISCIDUS_SLOWED_MORE = 26036
local SPELL_VISCIDUS_FREEZE      = 25937

local PHASE_FROST = 1 -- boss_viscidus.cpp:56
local PHASE_MELEE = 2 -- boss_viscidus.cpp:57
local SPELL_SCHOOL_MASK_FROST = 16 -- SharedDefines.h:328

local HITCOUNTER_SLOW      = 100 -- boss_viscidus.cpp:74
local HITCOUNTER_SLOW_MORE = 150 -- boss_viscidus.cpp:75
local HITCOUNTER_FREEZE    = 200 -- boss_viscidus.cpp:76
local HITCOUNTER_CRACK     = 50  -- boss_viscidus.cpp:78
local HITCOUNTER_SHATTER   = 100 -- boss_viscidus.cpp:79

-- Per-GUID _phase/_hitcounter (C++ Initialize() :109-116: 0/FROST).
local viscidusState = {}

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
    -- C++ Reset owns the init (Initialize :109-116); re-asserted here
    -- so a re-engage without an ev23 in between starts FROST/0.
    viscidusState[guid] = {phase = PHASE_FROST, hits = 0}
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
    -- C++ Reset -> Initialize() (boss_viscidus.cpp:109-111, :106-116).
    viscidusState[creature:GetGUIDLow()] = {phase = PHASE_FROST, hits = 0}
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
    viscidusState[creature:GetGUIDLow()] = nil
end

-- C++ SpellHit (boss_viscidus.cpp:154-181): frost-school hits in
-- PHASE_FROST above 5% hp advance the counter; >=100 slow,
-- >=150 slow-more, >=200 freeze into PHASE_MELEE with a 15s
-- reset (EVENT_RESET_PHASE :243-246).
local function onSpellHit(event, creature, caster, spellId)
    local guid = creature:GetGUIDLow()
    local st = viscidusState[guid]
    if st == nil or st.phase ~= PHASE_FROST then
        return
    end
    if bit32.band(creature:GetSpellSchoolMask(spellId), SPELL_SCHOOL_MASK_FROST) == 0 then
        return
    end
    if creature:GetHealthPct() <= 5.0 then
        return
    end
    st.hits = st.hits + 1
    if st.hits >= HITCOUNTER_FREEZE then
        st.hits = 0
        creature:Talk(2)
        st.phase = PHASE_MELEE
        creature:CastSpell(creature, SPELL_VISCIDUS_FREEZE)
        creature:RemoveAura(SPELL_VISCIDUS_SLOWED_MORE)
        schedule(guid, "resetPhase", 15000, function()
            local cur = viscidusState[guid]
            if cur then
                cur.hits = 0
                cur.phase = PHASE_FROST
            end
        end)
    elseif st.hits >= HITCOUNTER_SLOW_MORE then
        creature:Talk(1)
        creature:RemoveAura(SPELL_VISCIDUS_SLOWED)
        creature:CastSpell(creature, SPELL_VISCIDUS_SLOWED_MORE)
    elseif st.hits >= HITCOUNTER_SLOW then
        creature:Talk(0)
        creature:CastSpell(creature, SPELL_VISCIDUS_SLOWED)
    end
end

-- C++ DamageTaken (boss_viscidus.cpp:115-152): in PHASE_MELEE every
-- hit advances the same counter; ==50 crack, ==100 shatter. The
-- >=150 explode leg stays out (glob machine, see header).
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local st = viscidusState[guid]
    if st == nil or st.phase ~= PHASE_MELEE then
        return
    end
    st.hits = st.hits + 1
    if st.hits == HITCOUNTER_CRACK then
        creature:Talk(3)
    elseif st.hits == HITCOUNTER_SHATTER then
        creature:Talk(4)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 14, onSpellHit)
RegisterCreatureEvent(ENTRY, 23, onReset)
