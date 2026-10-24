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
-- Blow 54719); Reset-scheduled 10-30s init, rescheduled 5-15s.
-- JustEngagedWith — me->RemoveAura(SPELL_FREEZE_ANIM 16245) (:132)
-- rides the creature:RemoveAura bridge (the saurfang/arlokk idiom);
-- the Freeze Anim aura is cast only by the unported DoAction machine,
-- so in Go the removal is a faithful no-op that keeps the arm C++-
-- shaped for the day the freeze machine lands.
-- Unmodeled (no bridges — documented, not wired):
-- DamageTaken phase machine (HealthBelowPct(50/5) gates ARE bridgeable
-- via creature:GetHealthPct() — the sindragosa.lua:231 generic-fallback
-- precedent, so the old "no health-pct bridge" claim is stale; the
-- machine stays out because the payload is ACTION_FREEZE_COLOSSUS +
-- ACTION_SUMMON_ELEMENTAL (no DoAction bridge), SetImmuneToPC(true) /
-- REACT_PASSIVE / MoveIdle legs (the SetFlag(59)/SetReactState bridges
-- exist since 2026-10-20, but the legs ride the unported DoAction phase
-- machine, so they stay out), and
-- the damage=0 leg must not fire without the phase machine behind
-- it).
-- Constructor SetReactState(REACT_PASSIVE) (:90) — the AI
-- constructor has no Lua lifecycle equivalent (no ctor hook), so it
-- cannot ride an event; the passive-until-unfreeze story is the same
-- unported DoAction freeze machine documented below.
-- DoAction legs: ACTION_SUMMON_ELEMENTAL — DoCastSelf(Emerge
-- 54850) is bridged but has no Lua fire site (no DoAction bridge,
-- standing); ACTION_FREEZE_COLOSSUS — Clear/MoveIdle motion,
-- REACT_PASSIVE, SetImmuneToPC(true), DoCastSelf(Freeze Anim
-- 16245), the flag/react flips now bridgeable but still gated behind
-- the absent DoAction dispatch; ACTION_UNFREEZE_COLOSSUS — immune/react
-- flips + DoZoneInCombat behind the same absent bridges.
-- JustDied — _JustDied() instance bookkeeping only, no bridge.
-- Reset INTRO_DONE leg — cross-AI GetData/SetData bridge absent
-- (the Reset RemoveAura(SPELL_FREEZE_ANIM) rides the INTRO_DONE
-- gate, so it stays out with it).
-- JustSummoned DoZoneInCombat(summon) -- PORTED 2026-10-24
-- (colossusJustSummoned, ev19): the unconditional leg, C++-exact
-- (summoned:DoZoneInCombat() = CreatureAI::DoZoneInCombat(summon),
-- CreatureAI.cpp:73-103). The COLOSSUS_PHASE_SECOND_ELEMENTAL_SUMMON
-- half-health leg stays out (phase lives in the unported DoAction
-- machine, no phase latch in Lua). LATENT: no Lua path summons
-- elementals yet (DoAction summon machine unported, elemental entry
-- unverifiable) -- the arm fires correctly if a future path summons
-- via the colossus (anubrekhan latent precedent).
-- boss_drakkari_elemental — ENTRY UNVERIFIABLE (gundrak.h
-- GDCreatureIds has no NPC_DRAKKARI_ELEMENTAL; the .cpp enum names
-- only spells/texts; belnistrasz/willix precedent — no invented
-- identifiers): zero registration. Port-pattern-ready but
-- entry-blocked arms: EVENT_SURGE (5-15s) — DoCastSelf(Surge Visual
-- 54827) + randomPlayer-targeted DoCast(Surge 54801) (random-target
-- IS bridgeable — the archavon precedent — so the whole arm is
-- pattern-ready); JustDied Talk(EMOTE_ACTIVATE_ALTAR 1) (Talk is
-- bridgeable; the Unit::Kill(colossus) payload needs cross-instance
-- creature lookup, no bridge); DamageTaken HealthBelowPct(50) ->
-- cross-AI GetData(DATA_COLOSSUS_PHASE) + DoAction
-- (ACTION_RETURN_TO_COLOSSUS) (GetData/DoAction bridges absent);
-- EnterEvadeMode DespawnOrUnsummon — now bridgeable via
-- creature:Despawn (lua_creature_events.go — bridged this tick for
-- the vashj/morogrim/depth-charge nil-call time-bombs; the old
-- "no despawn bridge" claim is stale).
-- (boss_ai.go:1904), but the payload is cross-AI DoAction
-- (ACTION_UNFREEZE_COLOSSUS) + self-despawn (DoAction bridge
-- absent); Reset's AddAura(Mojo Volley 54849, self) — creature:
-- AddAura IS bridged (lua_creature_events.go:161), so the old "no
-- AddAura bridge" claim is stale. Joins the bridgeable-but-entry-
-- blocked queue.
-- npc_living_mojo — ENTRY UNVERIFIABLE (gundrak.h GDCreatureIds names
-- only colossus 29307 / slad_ran / moorabi / gal_darah / eck / misc;
-- no NPC_DRAKKARI_ELEMENTAL or NPC_LIVING_MOJO constant exists in the
-- C++ sources; belnistrasz/willix precedent): zero registration.
-- Port-pattern-ready but entry-blocked arms: JustEngagedWith
-- DoCastVictim(Mojo Wave 55626, 2s init, 15s repeat) +
-- DoCastVictim(Mojo Puddle 55627, 7s init, 18s repeat) — the exact
-- slad_ran RegisterCreatureEvent 1/2/23 pattern. The rest is
-- unbridgeable anyway: MoveMojos MovePoint choreography (movement
-- bridge absent), AttackStart home-proximity passive flip
-- (SetReactState IS bridged — stays out on the entry-blocked /
-- no-registration ground like the rest), MovementInform id-1 ->
-- cross-AI DoAction(UNFREEZE) + SetData(DATA_INTRO_DONE) +
-- DoZoneInCombat + DespawnOrUnsummon (the DoZoneInCombat bridge is live
-- 2026-10-23 and creature:Despawn IS bridged, but the whole chain rides
-- the unbridged DoAction/instance/MovementInform machine — stays out).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler
-- (gargolmar precedent); BossAI _Reset/_JustDied instance
-- bookkeeping has no bridge.

local ENTRY_DRAKKARI_COLOSSUS = 29307

local SPELL_MIGHTY_BLOW = 54719
local SPELL_FREEZE_ANIM = 16245

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

-- C++ :129-133 JustEngagedWith: BossAI leg unmodeled, then
-- me->RemoveAura(SPELL_FREEZE_ANIM) via the RemoveAura bridge.
local function colossusEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:RemoveAura(SPELL_FREEZE_ANIM)
    schedule(guid, "mightyblow", math.random(10000, 30000), function()
        mightyBlowTick(creature, guid)
    end)
end

local function colossusLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ EnterEvadeMode (boss_drakkari_colossus.cpp:328-331):
    -- me->DespawnOrUnsummon() — the colossus leaves the world and
    -- respawns on its normal delay via creature:Despawn.
    creature:Despawn()
end

local function colossusDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function colossusReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustSummoned (boss_drakkari_colossus.cpp:218-222): DoZoneInCombat(summon)
-- unconditionally; the phase-gated half-health leg stays out (see header).
local function colossusJustSummoned(_, creature, summoned)
    summoned:DoZoneInCombat()
end

RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 1, colossusEnterCombat)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 2, colossusLeaveCombat)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 4, colossusDied)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 23, colossusReset)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 19, colossusJustSummoned)
