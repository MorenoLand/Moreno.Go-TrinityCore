-- Prince Taldaram (Ahn'kahet) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/Ahnkahet/boss_prince_taldaram.cpp
-- (boss_prince_taldaram). Ahn'kahet dungeon-script unit per
-- northrend_script_loader.cpp order (decl 33 / call 223, immediately
-- after AddSC_boss_elder_nadox(); next: boss_amanitar).
-- Entry: 29308 Prince Taldaram (ahnkahet.h NPC_PRINCE_TALDARAM —
-- kalecgos pass; the GetAhnKahetAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(creature, spell) = DoCastSelf (moroes precedent).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler (gargolmar
-- precedent); BossAI _Reset/_JustDied instance bookkeeping has no bridge.
-- Ported arms (C++-exact for all modeled arms):
-- boss_prince_taldaram: JustEngagedWith Talk(SAY_AGGRO 2) (event 1);
-- EVENT_BLOODTHIRST DoCastSelf(Bloodthirst 55968), 10s init, 10s repeat;
-- EVENT_CONJURE_FLAME_SPHERES DoCast(GetVictim(), Conjure Flame Sphere
-- 55931), 5s init, 15s repeat (creature:GetVictim() — slad_ran viper
-- precedent; C++ casts on the victim with the "random target?" note, not
-- triggered); KilledUnit Talk(SAY_SLAY 3) player-gated (event 3,
-- victim:GetObjectType()=="Player" — nalorakk precedent); JustDied
-- Talk(SAY_DEATH 4) (event 4). The embrace-target latch on KilledUnit
-- (clearing _embraceTargetGUID) is unmodeled — the GUID bridge is absent.
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_VANISH (25-35s init/repeat): gated on
-- GetThreatManager().GetThreatListSize() > 1 — no threat-list bridge on
-- the Lua surface — and the leg drives SelectTarget(Random, 0, 100.0f,
-- true) for the embrace target — the random-target SelectTarget bridge is
-- absent (cairne/kazzak precedent) — so the whole vanish/feed machine
-- (EVENT_START_FEEDING Shadowstep 55966 + Embrace of the Vampyr 55959 +
-- Talk(SAY_FEED 5), EVENT_DONE_FEEDING, _embraceTargetGUID via
-- ObjectAccessor::GetUnit, RemoveAurasDueToSpell(VANISH 55964),
-- CastStop) is documented-only; emulating the timer alone would fire
-- Vanish with no feed target, failing the C++-exact port bar.
-- DamageTaken embrace-damage latch (_embraceTakenDamage vs
-- DATA_EMBRACE_DMG 20000 / heroic 40000 -> Clear + CastStop) — no
-- DamageTaken hook on the Lua surface (npc_unkor_the_ruthless
-- precedent) plus the IsHeroic gate has no difficulty bridge (kelidan
-- breaker precedent) — documented-only.
-- The UpdateAI vanish-evade check (HasAura(VANISH 55964) +
-- IsThreatListEmpty -> EnterEvadeMode) — threat-list + evade bridges
-- absent — documented-only.
-- Reset CheckSpheres()/RemovePrison()/SummonCreatureGroup legs (sphere
-- data DATA_SPHERE_1/2, prison flags, UNIT_FLAG_NOT_SELECTABLE removal,
-- NPC_JEDOGA_CONTROLLER despawn, SPELL_HOVER_FALL 60425, MoveLand, GO
-- platform DATA_PRINCE_TALDARAM_PLATFORM handling, Talk(SAY_WARNING 1))
-- and the ctor SetDisableGravity(true) — instance-script model absent
-- (standing blocker) and no gravity bridge — documented-only.
-- npc_prince_taldaram_flame_sphere (30106 / 31686 / 31687 — entries
-- verifiable from the file enums, kalecgos pass) — ENTRY-VERIFIABLE BUT
-- BRIDGE-BLOCKED, zero registration: Reset DoCastSelf triggered (Flame
-- Sphere Spawn Effect 55891 + Flame Sphere Visual 55928) is
-- port-pattern-ready (event 23 OnReset + DoCastSelf triggered — moorabi
-- precedent), but the whole functional AI is behind absent bridges —
-- SetGUID(_flameSphereTargetGUID) from the boss JustSummoned leg has no
-- cross-AI bridge, EVENT_START_MOVE's ObjectAccessor::GetUnit target
-- lookup + GetAbsoluteAngle MovePoint choreography has no motion-master
-- bridge, and EVENT_DESPAWN (DoCast 55947 + DespawnOrUnsummon) is behind
-- the despawn bridge (terestian precedent) — registering the two
-- cosmetic casts alone would leave the creature motionless and
-- non-despawning, failing the faithful-port bar.
-- go_prince_taldaram_sphere (193093 / 193094 Ancient Nerubian Device,
-- OnGossipHello: SetFlag NOT_SELECTABLE + SetGoState ACTIVE +
-- instance SetData(DATA_SPHERE_1/2) + boss Talk(SAY_1 0) +
-- CheckSpheres()) — no GameObjectAI binding bridge on the Lua surface
-- (go_crystal_prison precedent) plus the payload requires the absent
-- instance SetData model — documented-only.
-- spell_prince_taldaram_conjure_flame_sphere (55931: self-cast
-- 55895 + heroic 59511/59512) and
-- spell_prince_taldaram_flame_sphere_summon (55895/59511/59512:
-- destination Z offset) — no SpellScript binding bridge on the Lua
-- surface (razelikh precedent) — documented-only.
-- NOTE: the ICC Blood Prince Council's boss_prince_taldaram_icc is a
-- distinct registered script (boss_blood_prince_council.cpp, call 1349);
-- this lua covers only the Ahn'kahet boss_prince_taldaram registration.

local ENTRY_PRINCE_TALDARAM = 29308

local SAY_AGGRO = 2
local SAY_SLAY  = 3
local SAY_DEATH = 4

local SPELL_BLOODTHIRST          = 55968
local SPELL_CONJURE_FLAME_SPHERE = 55931

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

-- C++ EVENT_BLOODTHIRST: DoCastSelf; 10s init, 10s repeat.
local function bloodthirstTick(creature, guid)
    creature:CastSpell(creature, SPELL_BLOODTHIRST)
    schedule(guid, "bloodthirst", 10000, function()
        bloodthirstTick(creature, guid)
    end)
end

-- C++ EVENT_CONJURE_FLAME_SPHERES: DoCast(victim); 5s init, 15s repeat.
local function conjureTick(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CONJURE_FLAME_SPHERE)
    end
    schedule(guid, "conjure", 15000, function()
        conjureTick(creature, guid)
    end)
end

local function taldaramEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "bloodthirst", 10000, function()
        bloodthirstTick(creature, guid)
    end)
    schedule(guid, "conjure", 5000, function()
        conjureTick(creature, guid)
    end)
end

local function taldaramLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function taldaramTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function taldaramDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function taldaramReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 1, taldaramEnterCombat)
RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 2, taldaramLeaveCombat)
RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 3, taldaramTargetDied)
RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 4, taldaramDied)
RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 23, taldaramReset)
