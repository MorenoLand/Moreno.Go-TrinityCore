-- Drakkari Colossus (Gundrak) — Lua port of
-- src/server/scripts/Northrend/Gundrak/boss_drakkari_colossus.cpp
-- (boss_drakkari_colossus + boss_drakkari_elemental + npc_living_mojo).
-- Gundrak dungeon-script unit per northrend_script_loader.cpp order
-- (decl 22 / call 217, immediately after AddSC_boss_moorabi();
-- next: boss_gal_darah).
-- Entry: 29307 Drakkari Colossus (gundrak.h NPC_DRAKKARI_COLOSSUS —
-- the RegisterCreatureAIWithFactory(GetGundrakAI) ScriptName binding
-- is instance-shimmed, the creature_template binding DB-side as
-- usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell[, triggered]) = DoCastSelf
-- (phase_hunter / apothecary_hanes precedent — the trailing true is
-- the triggered port).
-- Ported arms (C++-exact for all modeled arms):
-- boss_drakkari_colossus: EVENT_MIGHTY_BLOW — DoCastVictim(Mighty
-- Blow 54719); Reset-scheduled 10-30s init, rescheduled 5-15s. The
-- only bridgeable arm of the slice.
-- Unmodeled (no bridges — documented, not wired):
-- DamageTaken phase machine (HealthBelowPct(50/5) gates — no
-- health-pct bridge (doomwalker precedent); the phase flip +
-- ACTION_FREEZE_COLOSSUS + ACTION_SUMMON_ELEMENTAL legs sit behind
-- the absent DoAction bridge, and the immunity/react/movement
-- legs behind SetImmuneToPC / SetReactState / motion-master
-- bridges).
-- DoAction legs: ACTION_SUMMON_ELEMENTAL — DoCastSelf(Emerge
-- 54850) is bridged but has no Lua fire site (no DoAction bridge,
-- standing); ACTION_FREEZE_COLOSSUS — Clear/MoveIdle motion,
-- REACT_PASSIVE, SetImmuneToPC(true), DoCastSelf(Freeze Anim
-- 16245), all unbridged; ACTION_UNFREEZE_COLOSSUS — immune/react
-- flips + DoZoneInCombat behind the same absent bridges.
-- JustEngagedWith — RemoveAura(SPELL_FREEZE_ANIM): aura-removal
-- bridge absent (aeranas precedent). JustDied — _JustDied()
-- instance bookkeeping only, no bridge. Reset INTRO_DONE leg —
-- cross-AI GetData/SetData bridge absent.
-- JustSummoned DoZoneInCombat(summon) — no summon bridge.
-- boss_drakkari_elemental — ENTRY UNVERIFIABLE (no
-- NPC_DRAKKARI_ELEMENTAL constant anywhere in the C++ sources;
-- enum only names spells/texts; belnistrasz/willix precedent — no
-- invented identifiers): zero registration. Port-pattern-ready but
-- entry-blocked arms: EVENT_SURGE (5-15s) — DoCastSelf(Surge
-- Visual 54827) + random-target DoCast(Surge 54801) (SelectTarget
-- Random bridge absent — cairne/kazzak precedent, so even the
-- cast leg is unmodelable); JustDied Talk(EMOTE_ACTIVATE_ALTAR 1)
-- + Unit::Kill(colossus) via instance->GetCreature — Talk bridged
-- but the kill payload is behind the absent cross-instance
-- creature lookup (entry-blocked anyway); DamageTaken HealthBelowPct
-- (50) -> cross-AI GetData(DATA_COLOSSUS_PHASE) + DoAction
-- (ACTION_RETURN_TO_COLOSSUS) (bridges absent); EnterEvadeMode
-- DespawnOrUnsummon (no despawn bridge — terestian precedent);
-- SpellHitTarget(54878 Merge) -> cross-AI DoAction
-- (ACTION_UNFREEZE_COLOSSUS) + self-despawn (event-15-never-fires
-- + bridges absent). Reset's AddAura(Mojo Volley 54849, self)
-- has no AddAura bridge. Joins the bridgeable-but-entry-blocked
-- queue.
-- npc_living_mojo — ENTRY UNVERIFIABLE (same file evidence — the
-- file names only texts/spells; the mojo's own entry is DB-side
-- only; belnistrasz/willix precedent): zero registration.
-- Port-pattern-ready but entry-blocked arms: JustEngagedWith
-- DoCastVictim(Mojo Wave 55626, 2s init, 15s repeat) +
-- DoCastVictim(Mojo Puddle 55627, 7s init, 18s repeat) — the exact
-- slad_ran RegisterCreatureEvent 1/2/23 pattern. The rest is
-- unbridgeable anyway: MoveMojos MovePoint choreography (movement
-- bridge absent), AttackStart home-proximity passive flip
-- (SetReactState bridge absent), MovementInform id-1 ->
-- cross-AI DoAction(UNFREEZE) + SetData(DATA_INTRO_DONE) +
-- DoZoneInCombat + DespawnOrUnsummon (bridges absent).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler
-- (gargolmar precedent); BossAI _Reset/_JustDied instance
-- bookkeeping has no bridge.

local ENTRY_DRAKKARI_COLOSSUS = 29307

local SPELL_MIGHTY_BLOW = 54719

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

-- C++ EVENT_MIGHTY_BLOW: Reset-scheduled 10-30s init, reschedule
-- 5-15s; DoCastVictim. UpdateAI returns early while casting — the
-- Go engine handles cast-gating, the timer just re-arms.
local function mightyBlowTick(creature, guid)
    creature:CastSpell(nil, SPELL_MIGHTY_BLOW)
    schedule(guid, "mightyblow", math.random(5000, 15000), function()
        mightyBlowTick(creature, guid)
    end)
end

local function colossusEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "mightyblow", math.random(10000, 30000), function()
        mightyBlowTick(creature, guid)
    end)
end

local function colossusLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function colossusDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function colossusReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 1, colossusEnterCombat)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 2, colossusLeaveCombat)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 4, colossusDied)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 23, colossusReset)
