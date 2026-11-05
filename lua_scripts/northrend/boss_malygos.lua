-- Malygos (Eye of Eternity) — Lua port of
-- src/server/scripts/Northrend/Nexus/EyeOfEternity/boss_malygos.cpp
-- (boss_malygos (CreatureScript) via
-- GetEyeOfEternityAI<boss_malygosAI> (BossAI, DATA_MALYGOS_EVENT);
-- npc_portal_eoe / npc_power_spark / npc_melee_hover_disk /
-- npc_caster_hover_disk / npc_nexus_lord / npc_scion_of_eternity /
-- npc_arcane_overload / npc_wyrmrest_skytalon / npc_static_field
-- (CreatureScripts); spell_malygos_portal_beam / spell_malygos_-
-- random_portal / spell_malygos_arcane_storm / spell_malygos_-
-- vortex_dummy / spell_malygos_vortex_visual / spell_arcane_-
-- overload / spell_nexus_lord_align_disk_aggro /
-- spell_scion_of_eternity_arcane_barrage /
-- spell_malygos_destroy_platform_channel /
-- spell_alexstrasza_bunny_destroy_platform_boom_visual /
-- spell_alexstrasza_bunny_destroy_platform_event /
-- spell_wyrmrest_skytalon_summon_red_dragon_buddy /
-- spell_wyrmrest_skytalon_ride_red_dragon_buddy_trigger /
-- spell_malygos_surge_of_power_warning_selector_25 /
-- spell_malygos_surge_of_power_25 / spell_alexstrasza_gift_beam /
-- spell_alexstrasza_gift_beam_visual (SpellScriptLoaders ->
-- SpellScript / AuraScript); achievement_denyin_the_scion
-- (AchievementCriteriaScript); all registered from inside
-- AddSC_boss_malygos(); loader decl 91 / call 286 per
-- northrend_script_loader.cpp — the FIRST group of the
-- "// The Nexus: Eye of Eternity" block in AddNorthrendScripts(),
-- immediately after AddSC_oculus() (decl 89 / call 284), under
-- the "// The Nexus: Eye of Eternity" marker; the call after it
-- is AddSC_instance_eye_of_eternity() (decl 92 / call 287) —
-- loader order confirmed this run; the checkpoint sequence
-- (oculus -> boss_malygos) is followed).
-- Entry: 28859 Malygos (eye_of_eternity.h NPC_MALYGOS, line 49;
-- DATA_MALYGOS_EVENT = 0, line 28;
-- instance_eye_of_eternity.cpp OnCreatureCreate binds case
-- NPC_MALYGOS (line 134) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of the
-- 28 script names hits boss_malygos.cpp (+ the loader decl/call
-- lines for AddSC_boss_malygos) only; zero sql/ hits. No malygos
-- lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4
-- OnDied. No timers in the ported arms; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_START_P_ONE 1) +
-- DoCast(SPELL_BERSERK 60670) (event 1; the setActive /
-- CheckRequiredBosses / EnterEvadeMode / SetBossState /
-- DoStartTimedAchievement legs have no bridges —
-- the tharon_ja BossAI-bookkeeping precedent). The berserk is a
-- non-triggered self-cast at engage (1-arg DoCast, UnitAI.cpp default
-- target self) -- the aura carries the 10-min timer itself.
-- KilledUnit — C++-GATED on who->GetTypeId() == TYPEID_PLAYER,
-- then Talk(SAY_KILLED_PLAYER_P_ONE 3 / P_TWO 8 / P_THREE 15)
-- selected by the AI's internal _phase (PHASE_ONE / PHASE_TWO /
-- PHASE_THREE) with a 5s kill-spam filter — DOCUMENTED-ONLY:
-- the phase-select has no bridge (no phase model in Lua), so no
-- faithful port exists.
-- npc_arcane_overload (NPC_ARCANE_OVERLOAD 30282, own file
-- npc_arcane_overload.lua, registered via
-- engine/world/boss_ai.go:RegisterLuaBoss("npc_arcane_overload",
-- 30282)): SpellHit SPELL_ARCANE_BOMB_TRIGGER (56430) ->
-- DoCastAOE SPELL_ARCANE_BOMB_KNOCKBACK_DAMAGE (56431, triggered) +
-- DoCastSelf SPELL_ARCANE_OVERLOAD_1 (56432, triggered), in C++ order
-- via event 14 (event, creature, caster, spellid) with the exact
-- triggered flags; gated on the trigger spell ID (the C++ SpellHit
-- if) — C++ boss_malygos.cpp :1478-:1484. Note: the engine fires event
-- 14 once per hit target — the true C++ DoDamageAndTriggers twin — so
-- this is 1:1 with the C++ per-hit SpellHit (the earlier per-effect
-- caveat is retracted).
-- JustDied — Talk(SAY_DEATH 17) (event 4; the _JustDied
-- bookkeeping has no bridge — tharon_ja precedent; the
-- GetGuidData DATA_GIFT_BOX_BUNNY_GUID -> SummonGameObject
-- GO_HEART_OF_MAGIC legs and the NPC_ALEXSTRASZA 32295 summon
-- are instance-resolution blocked — the GetGuidData leg has no
-- bridge; creature:SummonGameObject / creature:SummonCreature
-- are bridged (lua_creature_events.go:664/:774), but the summoner
-- itself is unresolvable without the instance bridge. Documented-only.
-- The 5s
-- DespawnOrUnsummon IS ported (malygosDied below — stale
-- "no despawn bridge" retracted; creature:Despawn() is bridged,
-- lua_objects.go).
-- Unmodeled (no bridges — documented, not wired):
-- Reset / Initialize (gravity / immunity / flags / flight-speed
-- legs, SetPhase(PHASE_NOT_STARTED), REACT_PASSIVE,
-- SetBossState NOT_STARTED) — the gravity leg + flag-clear +
-- REACT_PASSIVE legs are ported (malygosReset); immunity /
-- flight-speed / phase / instance legs stay out (no immune surface,
-- no speed literal, no phase / instance bridges). DoAction (the ~60-action event/phase
-- machine: ACTION_LAND_ENCOUNTER_START, platform-destroy
-- intro, vortex legs, surge-of-power legs, respawn handling —
-- all gated on MotionMaster / instance GUIDs / timed events) —
-- dispatch IS bridged (creature:DoAction fires ev38,
-- lua_creature_events.go:802); the machine stays out on its unported
-- drivers (motion / instance), not dispatch. MovementInform
-- (POINT_LAND_P_ONE / vortex takeoff-land / cyclic movement
-- points) — no motion bridge. boss_malygos.cpp:720 POINT_IDLE_P_THREE
-- SetControlled(true, UNIT_STATE_ROOT) rides that same unbridged
-- MovementInform: stays out. DamageTaken (surge-of-power and
-- destroy-platform health gates, immune flag flips) — no
-- health / timer-event bridges. UpdateAI (phase-one arcane
-- event machine, vortex handling, power-spark handling,
-- phase-three disk/scion machine) — no timer-event / cast /
-- phase bridges. SpellHit — SPELL_POWER_SPARK_MALYGOS 56152
-- (Talk SAY_BUFF_SPARK 14 + despawn the spark) stays out: the
-- trigger is Malygos's MoveInLineOfSight arm (event 27 exists,
-- but the arm is PHASE_ONE-gated with no phase model in Lua —
-- next bullet), so event 14 can never fire with this ID;
-- SPELL_MALYGOS_BERSERK 47008 -> Talk(EMOTE_HIT_BERSERKER_TIMER
-- 19) stays out: the trigger is the self-cast SPELL_BERSERK 60670
-- aura's 10-minute tick delivering 47008, but the Go creature-cast
-- path applies no aura effects ("no aura-application site at cast
-- completion", boss_ai.go) — the 60670 engage cast applies nothing,
-- so 47008 is never delivered and event 14 can never fire with
-- this ID (re-audit 2026-10-15: the 22:34 "trigger live in Go"
-- claim was wrong).
-- MoveInLineOfSight (power-spark proximity -> cast
-- SPELL_POWER_SPARK_MALYGOS) — event 27 exists, but the arm is
-- gated on _phase == PHASE_ONE (no phase model in Lua) and the
-- spark's approach movement is unported — stays out (2026-10-23
-- re-triage; the stale "no LOS bridge" wording is retracted).
-- npc_portal_eoe (NPC_PORTAL_TRIGGER 30118) — SpellHit
-- SPELL_PORTAL_OPENED -> phase-one-gated DoCast
-- SPELL_SUMMON_POWER_PARK; UpdateAI per-tick aura maintenance —
-- HasAura/RemoveAura/CastSpell are bridged (stale "no aura bridge"
-- re-keyed) but there is no per-tick UpdateAI bridge and the
-- DATA_PHASE gate is instance-bound (no instance bridge) —
-- documented-only. npc_power_spark
-- (NPC_POWER_SPARK 30084) — UpdateAI despawn-when-reached legs
-- documented-only (no motion bridge); JustDied casts
-- SPELL_POWER_SPARK_DEATH 55852 triggered self-cast PORTED as
-- lua_scripts/northrend/npc_power_spark.lua (event 4 — no phase
-- gate in the C++ arm). npc_melee_hover_disk (NPC_HOVER_DISK_MELEE 30234) /
-- npc_caster_hover_disk (NPC_HOVER_DISK_CASTER 30248) —
-- VehicleAI: PassengerBoarded (player enters -> welcome /
-- special-attack / take-off event machine; UNIT-type passenger
-- -> SPELL_TELEPORT_VISUAL_ONLY), MovementInform gravity
-- legs — no vehicle / passenger / motion bridges.
-- npc_nexus_lord (NPC_HOVER disk adds' lord) — DoAction ->
-- EVENT_NUKE_DUMMY / EVENT_ARCANE_SHOCK / EVENT_HASTE_BUFF
-- event machine (DoCast SPELL_ARCANE_SHOCK / haste, nuke the
-- disk the player rides) — DoAction-gated, but dispatch IS bridged
-- (creature:DoAction fires ev38, lua_creature_events.go:802): the
-- sender is the melee hover disk's MovementInform waypoint arrival
-- (boss_malygos.cpp:1222), an unported vehicle/motion machine, so
-- the machine never starts (stale "no timer-event bridge" re-keyed:
-- the timer bridge is live but the machine never starts) —
-- documented-only.
-- npc_scion_of_eternity (entry-UNVERIFIABLE from repo sources:
-- NPC_SURGE_OF_POWER 30334 is defined at eye_of_eternity.h :60 but
-- never referenced anywhere in the C++ tree; the ScriptName->entry
-- binding is DB-side — the npc_azure_ring_captain /
-- gothik-minions precedent) —
-- IsSummonedBy schedules EVENT_ARCANE_BARRAGE (target-list
-- arcane barrage machine), JustDied increments Malygos
-- DATA_SUMMON_DEATHS — no timer-event / instance /
-- target-selection bridges. npc_arcane_overload
-- (NPC_ARCANE_OVERLOAD 30282) — the SpellHit SPELL_ARCANE_BOMB_TRIGGER
-- -> SPELL_ARCANE_BOMB_KNOCKBACK_DAMAGE + SPELL_ARCANE_OVERLOAD_1
-- arm IS PORTED (event 14, own file npc_arcane_overload.lua — see
-- the Ported arms section above); IsSummonedBy -> SetGUID
-- DATA_LAST_OVERLOAD_GUID and the phase-gated DespawnOrUnsummon
-- legs — no summon / despawn / instance bridges. npc_wyrmrest_-
-- skytalon (NPC_WYRMREST_SKYTALON 30161) — VehicleAI phase-three
-- disk machine: IsSummonedBy -> MovePoint cycle; SpellHit
-- shock-lance legs; PassengerBoarded player machine — no
-- vehicle / motion / passenger bridges. npc_static_field
-- (entry-UNVERIFIABLE from repo sources — the 30090
-- attribution is WRONG: boss_malygos.cpp:1762 binds 30090 to
-- the vortex trigger vehicle, and the ScriptName->entry
-- binding is DB-side) — IsSummonedBy ->
-- DespawnOrUnsummon(30s) (no periodic logic in the C++) —
-- stays out: creature:Despawn() IS bridged, and ev5 DOES fire on
-- summoned creatures (motionFor fires ev23+ev5 on creation) — but the
-- entry is unverifiable from repo sources, so no registration can be
-- written, and the summoner-side event 19 cannot be gated without
-- the entry.
-- The 17 spell scripts (SpellScript / AuraScript handlers:
-- portal beam relocating, random portal target selection,
-- arcane storm target filters, vortex dummy destination
-- selection, vortex visual periodic, arcane overload
-- immunity-window application, nexus-lord align-disk aggro
-- transfer, scion arcane-barrage target-count scaling,
-- destroy-platform channel aura removal, alexstrasza bunny
-- platform boom visual + event, wyrmrest skytalon red-dragon
-- buddy summon + ride trigger, surge-of-power 25 warning
-- target selection + the surge itself, alexstrasza gift beam
-- + gift beam visual) — no SpellScript bridge (the varos
-- precedents) and no AuraScript bridge (the keristrasza
-- precedent); all 17 join the standing queues, except the gift
-- beam visual OnApply (PORTED tick 204, below) and the portal
-- beam (ported, below).
-- achievement_denyin_the_scion — OnCheck: player's vehicle
-- base entry == NPC_HOVER_DISK_MELEE (30234) — no achievement
-- bridge (the kelthuzad / thaddius precedent); joins the
-- unmodeled-achievement queue.

local ENTRY_MALYGOS = 28859
local NPC_ALEXSTRASZA = 32295 -- eye_of_eternity.h:57

local SAY_START_P_ONE = 1
local SAY_DEATH = 17

-- C++ SPELL_BERSERK = 60670 (boss_malygos.cpp :105): DoCast(SPELL_BERSERK)
-- in JustEngagedWith (:577) -- 2-arg DoCast, non-triggered self-cast
-- (UnitAI.cpp default target self); the periodic aura itself carries the
-- 10-minute berserk timer, so the faithful port casts it once at engage.
local SPELL_BERSERK = 60670
local SPELL_MALYGOS_PORTAL_BEAM = 56046 -- boss_malygos.cpp:102
local SPELL_PORTAL_OPENED = 61236 -- eye_of_eternity.h:84

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

-- C++ JustEngagedWith (boss_malygos.cpp :563-:579): me->setActive(true) +
-- instance->CheckRequiredBosses gate +
-- instance->SetBossState(IN_PROGRESS) + Talk(SAY_START_P_ONE)
-- + DoCast(SPELL_BERSERK) + DoStartTimedAchievement. The Talk and the
-- berserk self-cast are bridgeable; the instance legs are
-- BossAI-bookkeeping with no bridge (tharon_ja precedent).
local function malygosEnterCombat(event, creature, target)
    creature:Talk(SAY_START_P_ONE)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ JustDied (boss_malygos.cpp:979-987): _JustDied() +
-- Talk(SAY_DEATH) + gift-box-bunny GUID ->
-- SummonGameObject(HEART_OF_MAGIC) + SummonCreature
-- NPC_ALEXSTRASZA + DespawnOrUnsummon(5s). The Talk arm is
-- ported; the gift-box-bunny SummonGameObject leg stays out —
-- the summoner resolves via instance GetGuidData (no bridge).
-- The SummonCreature(NPC_ALEXSTRASZA 32295) leg IS ported
-- 2026-10-23 (summoner is me — no instance leg; AlexstraszaSpawnPos
-- boss_malygos.cpp:290; TEMPSUMMON_MANUAL_DESPAWN = 7, the bridge
-- default); Alexstrasza has no C++ AI script, so no behavior is
-- lost. The 5s despawn is a fire-and-forget one-shot
-- CreateLuaEvent — the creature is dead, so no timer bookkeeping
-- is needed (C++ DespawnOrUnsummon rides the creature's own
-- despawn timer, not the EventMap).
local function malygosDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    -- C++ arm order: the gift-box-bunny SummonGameObject leg is
    -- instance-blocked (above); the Alexstrasza summon lands here.
    creature:SummonCreature(NPC_ALEXSTRASZA, 854.551, 1225.31, 300.901, 0.0, 7, 0)
    CreateLuaEvent(function()
        creature:Despawn()
    end, 5000)
end

-- C++ Reset (boss_malygos.cpp:376-385): me->SetDisableGravity(true) --
-- unconditional leg of boss_malygosAI::Reset (Malygos hovers at spawn /
-- after a wipe). The RemoveFlag(NOT_SELECTABLE) + REACT_PASSIVE legs
-- are PORTED below (both surfaces bridged; C++ JustEngagedWith sets
-- no react state, so PASSIVE at Reset strands nothing beyond C++).
-- The flight-speed (member value, no literal), SetPhase, instance
-- boss-state/achievement legs stay out. The DoAction land clear for
-- the immune flag rides the unported land machine; the 0x100/0x200
-- bits have no Go consumer today, so the Reset apply below is a
-- faithful write with no behavior change until one lands.
local function malygosReset(event, creature)
    creature:SetDisableGravity(true)
    creature:SetFlag(59, 0x300) -- SetImmuneToAll(true), C++ order before the NOT_SELECTABLE clear
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetReactState(0) -- REACT_PASSIVE
end

RegisterCreatureEvent(ENTRY_MALYGOS, 1, malygosEnterCombat)
RegisterCreatureEvent(ENTRY_MALYGOS, 4, malygosDied)
RegisterCreatureEvent(ENTRY_MALYGOS, 23, malygosReset)

-- C++ spell_malygos_portal_beam::OnApply
-- (boss_malygos.cpp:1607-1611): the Load() gate (:1597-1600)
-- admits creature casters only, and the target must be a creature
-- (ToCreature); the portal target then casts SPELL_PORTAL_OPENED
-- (61236) on itself, non-triggered. The engine fires this hook
-- from the aura-apply path (RegisterAuraEvent, AuraEventOnApply =
-- 2; tick-196 hook). No PreventDefaultAction in C++.
local function portalBeamApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if target:GetObjectType() ~= "Creature" then
        return
    end
    target:CastSpell(target, SPELL_PORTAL_OPENED, false)
end

RegisterAuraEvent(SPELL_MALYGOS_PORTAL_BEAM, 2, portalBeamApply)

-- C++ spell_malygos_portal_beam::OnRemove
-- (boss_malygos.cpp:1613-1617): AfterEffectRemove — same Load /
-- ToCreature gates; the portal target removes SPELL_PORTAL_OPENED
-- from itself. The hook fires at the end of the remove funnel,
-- which is the AfterEffectRemove ordering.
local function portalBeamRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if target:GetObjectType() ~= "Creature" then
        return
    end
    target:RemoveAura(SPELL_PORTAL_OPENED)
end

RegisterAuraEvent(SPELL_MALYGOS_PORTAL_BEAM, 3, portalBeamRemove)

local SPELL_ALEXSTRASZAS_GIFT_BEAM_VISUAL = 61023 -- boss_malygos.cpp:157
local GO_ALEXSTRASZA_S_GIFT_10 = 193905 -- eye_of_eternity.h:69
local GO_ALEXSTRASZA_S_GIFT_25 = 193967 -- eye_of_eternity.h:70

-- C++ spell_alexstrasza_gift_beam_visual::OnApply
-- (boss_malygos.cpp:2380-2391): Load admits creature casters only;
-- the creature target summons Alexstrasza's Gift at its own position,
-- 10-man or 25-man by raid difficulty. STILL BLOCKED leg: OnRemove
-- clears NOT_SELECTABLE on the stored gift GO and on the Heart of
-- Magic GO via instance GetGuidData(DATA_HEART_OF_MAGIC_GUID) — no
-- instance bridge. The outer beam aura's own spell ID is DB-side only
-- (spell_script_names), so its apply (cast 61023) / remove (drop 61023)
-- legs cannot be registered.
local function alexstraszaGiftBeamVisualApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if target:GetObjectType() ~= "Creature" then
        return
    end
    local gift = GO_ALEXSTRASZA_S_GIFT_10
    if target:Is25ManRaid() then
        gift = GO_ALEXSTRASZA_S_GIFT_25
    end
    target:SummonGameObject(gift, target:GetX(), target:GetY(), target:GetZ(), target:GetO(), 0)
end

RegisterAuraEvent(SPELL_ALEXSTRASZAS_GIFT_BEAM_VISUAL, 2, alexstraszaGiftBeamVisualApply)
