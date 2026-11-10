-- Emerald Dragons (world bosses) — Lua port of
-- src/server/scripts/World/boss_emerald_dragons.cpp
-- (4 CreatureScripts: boss_ysondre / boss_lethon / boss_emeriss /
-- boss_taerar (WorldBossAI over the shared emerald_dragonAI base) +
-- npc_dream_fog (ScriptedAI, NPC_DREAM_FOG = 15224, zero Talk) +
-- npc_spirit_shade (PassiveAI, NPC_SPIRIT_SHADE = 15261, zero Talk);
-- 2 SpellScriptLoaders (spell_dream_fog_sleep, spell_mark_of_nature);
-- all registered from inside AddSC_emerald_dragons() (line 808);
-- loader decl 23 / call 44 per world_script_loader.cpp — the group
-- immediately after AddSC_areatrigger_scripts() (call 43) in
-- AddWorldScripts() — verified from the loader this run; the
-- checkpoint sequence (areatrigger_scripts -> emerald_dragons) is
-- followed).
-- Entries: 14887 Ysondre (Feralas) / 14888 Lethon (Duskwood) /
-- 14889 Emeriss (Hinterlands) / 14890 Taerar (Ashenvale) — all from
-- the file's own EmeraldDragonNPC enum; NPC_DREAM_FOG = 15224 from the
-- same enum; the ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_emerald_dragons" hits boss_emerald_dragons.cpp only (+ the
-- loader decl/call lines). No emerald-dragons lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 22 OnSummoned, 23 OnReset. Timers via CreateLuaEvent.
-- Ported arms (C++-exact for all modeled arms) — the four
-- JustEngagedWith arms (event 1 — the auriaya precedent; the
-- WorldBossAI::JustEngagedWith legs have no bridges):
-- boss_ysondre JustEngagedWith — Talk(SAY_YSONDRE_AGGRO 0) +
-- EVENT_LIGHTNING_WAVE (DoCastVictim 24819, 12s init,
-- randtime(10s,20s) repeat);
-- boss_lethon JustEngagedWith — Talk(SAY_LETHON_AGGRO 0) +
-- EVENT_SHADOW_BOLT_WHIRL (me->CastSpell(nullptr, 24834, false),
-- non-triggered, 10s init, randtime(15s,30s) repeat — lowered to
-- self-cast);
-- boss_emeriss JustEngagedWith — Talk(SAY_EMERISS_AGGRO 0) +
-- EVENT_VOLATILE_INFECTION (DoCastVictim 24928, 12s init, 120s
-- repeat);
-- boss_taerar JustEngagedWith — Talk(SAY_TAERAR_AGGRO 0) +
-- EVENT_ARCANE_BLAST (DoCast 24857 1-arg — AISpellInfo resolves to
-- AITARGET_VICTIM for the ranged nuke, so victim-cast; 12s init,
-- randtime(7s,12s) repeat) + EVENT_BELLOWING_ROAR (DoCast 22686 1-arg
-- — AISpellInfo stays AITARGET_SELF for the range-0 PBAOE, so
-- self-cast; 30s init, randtime(20s,30s) repeat).
-- (The Oct-12 "no-timer bridge" claim was wrong — CreateLuaEvent is
-- a real bridge, retracted this run.) All timers cancelled on
-- 2/4/23 (gargolmar precedent).
-- PORTED (2026-10-19): boss_taerarAI::Reset's RemoveAurasDueToSpell(
-- SPELL_SHADE 24313) — taerarReset; C++-exact (apply leg unported, so a
-- no-op today).
-- DOCUMENTED-ONLY (in this header; only the four dragon entries
-- registered):
-- emerald_dragonAI Reset — PORTED 2026-10-24: RemoveFlag NOT_SELECTABLE /
-- NON_ATTACKABLE (already ported 2026-10-20) + DoCast(me, SPELL_MARK_OF_
-- NATURE_AURA 25041, triggered) self-cast in dragonReset + SetReactState
-- (REACT_AGGRESSIVE) :101 — the banish machine's REACT_PASSIVE :639 can
-- still be live at Reset (evade mid-banish), so the leg is ported; the
-- aura is mechanically inert in Go (its 10s tick triggers 25042 via the
-- SpellScript-only spell_mark_of_nature), like the ported KilledUnit
-- 25040 cast.
-- The base EVENT_TAIL_SWEEP / EVENT_NOXIOUS_BREATH / EVENT_SEEPING_FOG
-- machine is PORTED 2026-10-24 (tailSweepTick / noxiousBreathTick /
-- seepingFogTick, seeded in all four EnterCombat handlers): the old
-- claim that the events ride the SPELL_SUMMON_PLAYER 24776 leg was
-- wrong — 24776 is a separate UpdateAI leg (:158-159), not part of
-- ExecuteEvent. That leg is PORTED 2026-10-24 too (summonPlayerTick,
-- 1s combat pump): MaxThreat player beyond 50yd via the threat-ordered
-- GetAITargets list + the reach-adjusted distance bar (UnitAI.cpp
-- DefaultTargetSelector: negative dist excludes in-range targets).
-- The three events are plain self-casts with their own re-arms.
-- emerald_dragonAI KilledUnit — PORTED (this run): player-gated
-- who->CastSpell(who, SPELL_MARK_OF_NATURE 25040, triggered), lowered to
-- creature:CastSpell(victim, 25040, true) (maiden_of_virtue / moroes
-- triggered-cast convention); the aura lands on the same target.
-- Mechanically inert in Go (no spell_mark_of_nature effect), like the
-- ported DamageTaken visual casts in this file — the old
-- "victim-self-cast / no-caster-override" non-port rationale is
-- retracted.
-- boss_emeriss KilledUnit — PORTED (this run): DoCast(who, SPELL_PUTRID_
-- MUSHROOM 24904, triggered) + the base mark leg, same convention.
-- boss_ysondre / boss_lethon / boss_emeriss DamageTaken — PORTED
-- (this run): OnDamageTaken event 9 is bridged (midnight never-die
-- clamp precedent), so the stale "no-DamageTaken bridge" claims are
-- retracted — Talk + cast + ++_stage, pre-damage health (event 9 fires
-- pre-damage, moroes precedent), C++-exact floor comparisons
-- (!HealthAbovePct(p) = health <= floor(max*p/100)). The three cast
-- spells (24795/24811/24910) have no Go effect implementation —
-- visual-only casts like midnight's SPELL_SUMMON_ATTUMEN.
-- boss_lethon SpellHitTarget — PORTED (lethonSpellHitTarget, ev15): the
-- stale "no-SpellHitTarget bridge" claim was retracted 2026-10-21 (ev15
-- CREATURE_EVENT_ON_SPELL_HIT_TARGET bridged, runtime.go:228; fired
-- boss_ai.go:2280-2287) and the stale "no summon bridge" claim retracted
-- 2026-10-23 (creature:SummonCreature live, lua_creature_events.go:728):
-- DRAW_SPIRIT hit-target -> SummonCreature(NPC_SPIRIT_SHADE 15261) at the
-- target's position, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT Eluna type 3,
-- 50s, with the C++ TYPEID_PLAYER gate.
-- boss_taerar DamageTaken — PORTED (this run): the 25%-per-stage
-- banish machine (boss_emerald_dragons.cpp:618-643) — InterruptNonMeleeSpells
-- (false) + AttackStop + Talk(SAY_TAERAR_SUMMON_SHADES 1) + 3x triggered
-- victim-cast shade summons 24841/24842/24843 + DoCast SPELL_SHADE 24313 +
-- NOT_SELECTABLE / NON_ATTACKABLE flags + REACT_PASSIVE + 60s banish timer
-- + SummonedCreatureDies _shades-- early exit (event 21). The stale
-- "no-InterruptNonMeleeSpells bridge (verified STAND 2026-10-23)" claim
-- is retracted: the bridge is live (lua_creature_events.go:1995,
-- baltharus precedent this sweep) and the flag/react/AttackStop/ev21
-- bridges were already live. The shade summons themselves are
-- triggered victim casts like the ported ysondre/lethon visual casts
-- (their Go spawn effect is unmodeled, as with lethon's draw-spirit
-- shade); the _shades count therefore only ever decrements on real
-- summoned-creature deaths, and the 60s timer is the guaranteed exit.
-- Timer ticks defer 500ms while banished (C++ skips the base UpdateAI
-- event loop while banished; the defer re-checks on exit).
-- The four dragon ExecuteEvent arms ported above (EVENT_LIGHTNING_WAVE
-- / EVENT_SHADOW_BOLT_WHIRL / EVENT_VOLATILE_INFECTION /
-- EVENT_ARCANE_BLAST / EVENT_BELLOWING_ROAR).
-- npc_dream_fog — zero Talk; NOT registered: the _roamTimer 15-30s
-- chase / 2.5s MoveRandom + MoveChase + SetWalk + walk-speed machine
-- — re-keyed 2026-10-15 (timer doctrine): no-movement bridge is the
-- true blocker (the timer leg bridges); with no registered Lua file
-- the class stays out. The stale "no-timer" claim is corrected.
-- npc_spirit_shade — zero Talk; PORTED: the IsSummonedBy
-- MoveFollow leg (boss_emerald_dragons.cpp:430-438) via ev22, plus the
-- MovementInform arrival leg (:440-446: dark-offering cast + 1s
-- DespawnOrUnsummon) on event 28 (spiritShadeMovementInform, tick
-- 393 — the proximity poll it replaced is retired). The stale
-- "no-movement / no-summon-death / no-despawn bridges" claim is
-- retracted: MoveFollow (lua_creature_events.go:4636) and
-- DespawnOrUnsummon (:6121) are both bridged, and no summon-death
-- arm exists on this class.
-- spell_dream_fog_sleep + spell_mark_of_nature (SpellScriptLoaders) —
-- no-SpellScript bridge — NOT registered.
-- All Talk() calls in the file accounted for (4 ported above +
-- SAY_YSONDRE_SUMMON_DRUIDS 1 + SAY_LETHON_DRAW_SPIRIT 1 +
-- SAY_EMERISS_CAST_CORRUPTION 1 + SAY_TAERAR_SUMMON_SHADES 1
-- documented above).

local ENTRY_YSONDRE = 14887
local ENTRY_LETHON  = 14888
local ENTRY_EMERISS = 14889
local ENTRY_TAERAR  = 14890

local ENTRY_SPIRIT_SHADE = 15261 -- NPC_SPIRIT_SHADE, boss_emerald_dragons.cpp:388

-- UnitDefines.h:125/149; SetFlag/RemoveFlag on UNIT_FIELD_FLAGS (59)
-- is bridged (engine/world/lua_creature_events.go:690/725).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

-- UnitDefines.h: REACT_PASSIVE=0, REACT_DEFENSIVE=1, REACT_AGGRESSIVE=2
-- (bridge comment lua_creature_events.go:3883-3889). Taerar's banish
-- machine calls SetReactState at both ends; the consts were undefined
-- globals before tick 299, aborting the handler before the 60s exit
-- was scheduled (permanent banish).
local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

local SAY_AGGRO = 0 -- SAY_YSONDRE_AGGRO / SAY_LETHON_AGGRO /
                    -- SAY_EMERISS_AGGRO / SAY_TAERAR_AGGRO (all 0)

local SPELL_LIGHTNING_WAVE = 24819 -- boss_emerald_dragons.cpp:248 (Ysondre)
local SPELL_SHADOW_BOLT_WHIRL = 24834 -- boss_emerald_dragons.cpp:337 (Lethon)
local SPELL_VOLATILE_INFECTION = 24928 -- boss_emerald_dragons.cpp:475 (Emeriss)
local SPELL_ARCANE_BLAST = 24857 -- boss_emerald_dragons.cpp:568 (Taerar)
local SPELL_BELLOWING_ROAR = 22686 -- boss_emerald_dragons.cpp:563 (Taerar)
local SPELL_SHADE = 24313 -- boss_emerald_dragons.cpp:563 (Taerar self-cast
    -- in the 25%-stage banish machine; cleared by boss_taerarAI::Reset)
local SPELL_SUMMON_SHADE_1 = 24841 -- boss_emerald_dragons.cpp:566
local SPELL_SUMMON_SHADE_2 = 24842 -- boss_emerald_dragons.cpp:567
local SPELL_SUMMON_SHADE_3 = 24843 -- boss_emerald_dragons.cpp:568
local SPELL_SUMMON_DRUID_SPIRITS = 24795 -- boss_emerald_dragons.cpp:249 (Ysondre DamageTaken)
local SPELL_DRAW_SPIRIT = 24811 -- boss_emerald_dragons.cpp:336 (Lethon DamageTaken)
local SPELL_DARK_OFFERING = 24804 -- boss_emerald_dragons.cpp:338 (spirit-shade MovementInform)
local SPELL_CORRUPTION_OF_EARTH = 24910 -- boss_emerald_dragons.cpp:474 (Emeriss DamageTaken)
local SPELL_MARK_OF_NATURE = 25040 -- boss_emerald_dragons.cpp:53,112 (all-dragon KilledUnit)
local SPELL_TAIL_SWEEP = 15847 -- boss_emerald_dragons.cpp:46 (base ExecuteEvent)
local SPELL_NOXIOUS_BREATH = 24818 -- boss_emerald_dragons.cpp:52 (base ExecuteEvent)
local SPELL_SEEPING_FOG_LEFT = 24813 -- boss_emerald_dragons.cpp:50 (base ExecuteEvent)
local SPELL_SEEPING_FOG_RIGHT = 24814 -- boss_emerald_dragons.cpp:51 (base ExecuteEvent)
local SPELL_MARK_OF_NATURE_AURA = 25041 -- boss_emerald_dragons.cpp:54 (base Reset)
local SPELL_SUMMON_PLAYER = 24776 -- boss_emerald_dragons.cpp:47 (base UpdateAI)
local SPELL_PUTRID_MUSHROOM = 24904 -- boss_emerald_dragons.cpp:473,505 (Emeriss KilledUnit)

local SAY_SUMMON = 1 -- SAY_YSONDRE_SUMMON_DRUIDS / SAY_LETHON_DRAW_SPIRIT /
                     -- SAY_EMERISS_CAST_CORRUPTION / SAY_TAERAR_SUMMON_SHADES (all 1)

-- per-GUID C++ _stage (Initialize/Reset sets 1; stages at 75/50/25%).
local stages = {}

-- per-GUID C++ _banished / _shades for boss_taerar's banish machine.
local taerarBanished = {}
local taerarShades = {}

local timers = {}

-- per-shade summoner GUID counter for the spirit-shade MovementInform
-- data comparison (C++ _summonerGuid.GetCounter()).
local shadeSummonerLow = {}

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

-- UpdateAI CASTING gates (emerald_dragonAI::UpdateAI :147 pre-loop +
-- :154 in-loop; taerar delegates to the base after its banished check)
-- ported as 500ms IsCasting defer on all 9 timer ticks (mal_ganis
-- idiom): the gate precedes target picks, casts and summons, and no
-- re-arm is consumed; the post-loop summon-player leg defers the same
-- way. DamageTaken / KilledUnit / Reset hooks ungated per C++
-- (separate hooks); dream-fog / spirit-shade unregistered, no tick.
-- C++ boss_ysondre::ExecuteEvent (:299-301): DoCastVictim(SPELL_
-- LIGHTNING_WAVE 24819) 2-arg (non-triggered); 12s init,
-- randtime(10s,20s) repeat.
local function lightningWaveTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "lightning_wave", 500, function()
            lightningWaveTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_LIGHTNING_WAVE)
    schedule(guid, "lightning_wave", {10000, 20000}, function()
        lightningWaveTick(creature, guid)
    end)
end

-- C++ boss_lethon::ExecuteEvent (:399-401): me->CastSpell(nullptr,
-- SPELL_SHADOW_BOLT_WHIRL 24834, false) — explicit non-triggered,
-- no explicit target; 10s init, randtime(15s,30s) repeat. Lowered to
-- self-cast creature:CastSpell(creature, spell) (boss_ai.go DoCastAOE
-- self-cast convention — selin_fireheart / mr_smite precedent).
local function shadowBoltWhirlTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadow_bolt_whirl", 500, function()
            shadowBoltWhirlTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SHADOW_BOLT_WHIRL)
    schedule(guid, "shadow_bolt_whirl", {15000, 30000}, function()
        shadowBoltWhirlTick(creature, guid)
    end)
end

-- C++ boss_emeriss::ExecuteEvent (:529-531): DoCastVictim(SPELL_
-- VOLATILE_INFECTION 24928) 2-arg (non-triggered); 12s init, 120s
-- repeat.
local function volatileInfectionTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "volatile_infection", 500, function()
            volatileInfectionTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_VOLATILE_INFECTION)
    schedule(guid, "volatile_infection", 120000, function()
        volatileInfectionTick(creature, guid)
    end)
end

-- C++ boss_taerar::ExecuteEvent (:649-651): DoCast(SPELL_ARCANE_BLAST
-- 24857) 1-arg (DoCastVictim); 12s init, randtime(7s,12s) repeat.
local function arcaneBlastTick(creature, guid)
    if taerarBanished[guid] then
        schedule(guid, "arcane_blast", 500, function()
            arcaneBlastTick(creature, guid)
        end)
        return
    end
    if creature:IsCasting() then
        schedule(guid, "arcane_blast", 500, function()
            arcaneBlastTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_ARCANE_BLAST)
    schedule(guid, "arcane_blast", {7000, 12000}, function()
        arcaneBlastTick(creature, guid)
    end)
end

-- C++ boss_taerar::ExecuteEvent (:653-655): DoCast(SPELL_BELLOWING_
-- ROAR 22686) 1-arg. The 1-arg DoCast resolves via AISpellInfo
-- (UnitAI.cpp FillAISpellInfo): Bellowing Roar is a PBAOE with range
-- 0, so the target stays AITARGET_SELF — self-cast. 30s init,
-- randtime(20s,30s) repeat.
local function bellowingRoarTick(creature, guid)
    if taerarBanished[guid] then
        schedule(guid, "bellowing_roar", 500, function()
            bellowingRoarTick(creature, guid)
        end)
        return
    end
    if creature:IsCasting() then
        schedule(guid, "bellowing_roar", 500, function()
            bellowingRoarTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BELLOWING_ROAR)
    schedule(guid, "bellowing_roar", {20000, 30000}, function()
        bellowingRoarTick(creature, guid)
    end)
end

-- C++ emerald_dragonAI::ExecuteEvent (:132-135): DoCast(me, SPELL_TAIL_
-- SWEEP 15847) 2-arg (non-triggered); 4s init, 2s repeat. All four
-- dragons fall through to the base ExecuteEvent for this event.
local function tailSweepTick(creature, guid)
    if taerarBanished[guid] then
        schedule(guid, "tail_sweep", 500, function()
            tailSweepTick(creature, guid)
        end)
        return
    end
    if creature:IsCasting() then
        schedule(guid, "tail_sweep", 500, function()
            tailSweepTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_TAIL_SWEEP)
    schedule(guid, "tail_sweep", 2000, function()
        tailSweepTick(creature, guid)
    end)
end

-- C++ emerald_dragonAI::ExecuteEvent (:127-130): DoCast(me, SPELL_NOXIOUS_
-- BREATH 24818) 2-arg (non-triggered); randtime(7.5s,15s) init/repeat.
local function noxiousBreathTick(creature, guid)
    if taerarBanished[guid] then
        schedule(guid, "noxious_breath", 500, function()
            noxiousBreathTick(creature, guid)
        end)
        return
    end
    if creature:IsCasting() then
        schedule(guid, "noxious_breath", 500, function()
            noxiousBreathTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_NOXIOUS_BREATH)
    schedule(guid, "noxious_breath", {7500, 15000}, function()
        noxiousBreathTick(creature, guid)
    end)
end

-- C++ emerald_dragonAI::ExecuteEvent (:120-125): DoCast(me, SPELL_SEEPING_
-- FOG_LEFT 24813, triggered) + DoCast(me, SPELL_SEEPING_FOG_RIGHT
-- 24814, triggered); randtime(12.5s,20s) init, randtime(120s,150s)
-- repeat. The fog-pair casts are self-casts; the summoned dream-fog
-- creatures' own AI stays unported (motion machine, see below).
local function seepingFogTick(creature, guid)
    if taerarBanished[guid] then
        schedule(guid, "seeping_fog", 500, function()
            seepingFogTick(creature, guid)
        end)
        return
    end
    if creature:IsCasting() then
        schedule(guid, "seeping_fog", 500, function()
            seepingFogTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SEEPING_FOG_LEFT, true)
    creature:CastSpell(creature, SPELL_SEEPING_FOG_RIGHT, true)
    schedule(guid, "seeping_fog", {120000, 150000}, function()
        seepingFogTick(creature, guid)
    end)
end

-- C++ emerald_dragonAI::UpdateAI (:158-159): SelectTarget(MaxThreat, 0,
-- -50.0f, playerOnly) -> DoCast(target, SPELL_SUMMON_PLAYER 24776)
-- 2-arg (non-triggered), every UpdateAI tick while the condition holds.
-- DefaultTargetSelector (UnitAI.cpp:259-279): a negative dist EXCLUDES
-- targets within 50yd — only the top-threat player BEYOND 50yd
-- (plus combat reaches) qualifies, i.e. a player wandering off.
-- Ported as a 1s combat pump (the per-tick check has no Lua event;
-- 1s is the file's pump granularity): iterate the threat-ordered
-- GetAITargets list (lua_creature_events.go:345), skipping
-- in-range players (C++ filters before picking), and cast when the
-- raw 3D GetDistance meets the C++
-- IsWithinCombatRange bar — exactDist >= 50 + dragon reach +
-- player reach, with the player reach taken as the 1.5 default
-- (combatReachOrDefault) since the player surface carries no
-- GetCombatReach read. C++ fires per tick; the 1s pump is coarser
-- but converges on the same teleport once the player stays out.
local function summonPlayerTick(creature, guid)
    if taerarBanished[guid] then
        schedule(guid, "summon_player", 500, function()
            summonPlayerTick(creature, guid)
        end)
        return
    end
    if creature:IsCasting() then
        schedule(guid, "summon_player", 500, function()
            summonPlayerTick(creature, guid)
        end)
        return
    end
    local targets = creature:GetAITargets()
    if targets then
        -- C++ filters FIRST (DefaultTargetSelector drops in-range
        -- targets), then picks MaxThreat among survivors: skip
        -- in-range players, cast on the first player beyond the bar.
        for _, target in ipairs(targets) do
            if target:GetObjectType() == "Player" and not target:IsDead() then
                local bar = 50 + creature:GetCombatReach() + 1.5
                if creature:GetDistance(target) >= bar then
                    creature:CastSpell(target, SPELL_SUMMON_PLAYER)
                    break
                end
            end
        end
    end
    schedule(guid, "summon_player", 1000, function()
        summonPlayerTick(creature, guid)
    end)
end

-- C++ boss_ysondre::DamageTaken (:283-293): if (!HealthAbovePct(100 -
-- 25*_stage)) { Talk(1); 10x DoCast(me, 24795, triggered); ++_stage; }.
-- Pre-damage health in the event-9 hook (moroes precedent); C++-exact
-- floor form (ironaya convention); health > 0 hook guard (midnight).
local function ysondreDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local stage = stages[guid] or 1
    stages[guid] = stage
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 or health == 0 then
        return
    end
    if health <= math.floor(maxHealth * (100 - 25 * stage) / 100) then
        creature:Talk(SAY_SUMMON)
        for _ = 1, 10 do
            creature:CastSpell(creature, SPELL_SUMMON_DRUID_SPIRITS, true)
        end
        stages[guid] = stage + 1
    end
end

-- C++ boss_lethon::DamageTaken (:376-384): if (!HealthAbovePct(100 -
-- 25*_stage)) { Talk(1); DoCast(me, 24811) non-triggered; ++_stage; }.
-- The SpellHitTarget spirit-shade summon is PORTED (lethonSpellHitTarget,
-- ev15 — both bridges now live); the draw-spirit cast itself is
-- visual-only (no Go effect for 24811).
local function lethonDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local stage = stages[guid] or 1
    stages[guid] = stage
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 or health == 0 then
        return
    end
    if health <= math.floor(maxHealth * (100 - 25 * stage) / 100) then
        creature:Talk(SAY_SUMMON)
        creature:CastSpell(creature, SPELL_DRAW_SPIRIT)
        stages[guid] = stage + 1
    end
end

-- C++ boss_lethon::SpellHitTarget (boss_emerald_dragons.cpp:386-393):
-- if (spellInfo->Id == SPELL_DRAW_SPIRIT && target->GetTypeId() ==
-- TYPEID_PLAYER) { Position targetPos = target->GetPosition();
-- me->SummonCreature(NPC_SPIRIT_SHADE, targetPos,
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 50s); } — gruul hook-15
-- precedent; Eluna summon type 3 = TIMED_DESPAWN_OUT_OF_COMBAT.
local function lethonSpellHitTarget(event, creature, target, spellId)
    if spellId == SPELL_DRAW_SPIRIT and target and
            target:GetObjectType() == "Player" then
        creature:SummonCreature(ENTRY_SPIRIT_SHADE, target:GetX(), target:GetY(),
            target:GetZ(), target:GetO(), 3, 50000)
    end
end

-- C++ npc_spirit_shadeAI::MovementInform (boss_emerald_dragons.cpp:
-- 440-446, event 28): if (moveType == FOLLOW_MOTION_TYPE && data ==
-- _summonerGuid.GetCounter()) { CastSpell(nullptr, SPELL_DARK_OFFERING,
-- false); DespawnOrUnsummon(1s); }. The Go bridge fires ev28 with
-- (FOLLOW_MOTION_TYPE 14, uint32(targetGUID)) once, when the follow
-- leg closes in range (engine/world/creaturemotion.go:4386). The
-- ev28 id is the low 32 bits of the GUID while GetGUIDLow is the
-- low 24, so the comparison masks with % 2^24 (exact in doubles
-- below 2^32); the shade only ever follows its summoner, so the
-- masked comparison is the C++ data check.
local FOLLOW_MOTION_TYPE = 14

local function spiritShadeMovementInform(_, creature, moveType, id)
    if moveType ~= FOLLOW_MOTION_TYPE then
        return
    end
    local expected = shadeSummonerLow[creature:GetGUIDLow()]
    if expected ~= nil and id % 16777216 == expected then
        creature:CastSpell(creature, SPELL_DARK_OFFERING)
        creature:DespawnOrUnsummon(1000)
    end
end

-- C++ npc_spirit_shadeAI::IsSummonedBy (boss_emerald_dragons.cpp:430-
-- 438): summoner ToUnit() non-nil -> store GUID, MoveFollow(summoner,
-- 0, 0).
local function spiritShadeSummoned(event, creature, summoner)
    if not summoner then
        return
    end
    shadeSummonerLow[creature:GetGUIDLow()] = summoner:GetGUIDLow()
    creature:MoveFollow(summoner, 0, 0)
end

local function spiritShadeCleanup(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    shadeSummonerLow[guid] = nil
end

-- C++ boss_emeriss::DamageTaken (:515-523): if (!HealthAbovePct(100 -
-- 25*_stage)) { Talk(1); DoCast(me, 24910, triggered); ++_stage; }.
local function emerissDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local stage = stages[guid] or 1
    stages[guid] = stage
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 or health == 0 then
        return
    end
    if health <= math.floor(maxHealth * (100 - 25 * stage) / 100) then
        creature:Talk(SAY_SUMMON)
        creature:CastSpell(creature, SPELL_CORRUPTION_OF_EARTH, true)
        stages[guid] = stage + 1
    end
end

-- C++ boss_taerarAI::UpdateAI (:663, exit legs :673-677) banish exit: RemoveFlag +
-- RemoveAurasDueToSpell(SPELL_SHADE) + SetReactState(REACT_AGGRESSIVE).
-- Called by the 60s banish timer and by the SummonedCreatureDies
-- early exit below; cancels the pending timer when called early
-- (ragnaros emerge precedent).
local function taerarEndBanish(creature, guid)
    taerarBanished[guid] = nil
    taerarShades[guid] = nil
    local per = timers[guid]
    if per and per["taerar_banish"] then
        RemoveEventById(per["taerar_banish"])
        per["taerar_banish"] = nil
    end
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_NON_ATTACKABLE)
    creature:RemoveAura(SPELL_SHADE)
    creature:SetReactState(REACT_AGGRESSIVE)
end

-- C++ boss_taerarAI::DamageTaken (:618-643): if (!_banished &&
-- !HealthAbovePct(100 - 25*_stage)) { _banished = true; 60s timer;
-- InterruptNonMeleeSpells(false); DoStopAttack; Talk(1); 3x
-- DoCastVictim(shade summons, triggered); _shades += 3; DoCast(SHADE);
-- SetFlag(NOT_SELECTABLE|NON_ATTACKABLE); SetReactState(PASSIVE);
-- ++_stage; }. C++ arm order kept. Pre-damage health + floor form
-- (sibling-dragon convention); the _banished latch is the re-trigger
-- guard (C++ leaves it set until the banish exit).
local function taerarDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local stage = stages[guid] or 1
    stages[guid] = stage
    if taerarBanished[guid] then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 or health == 0 then
        return
    end
    if health <= math.floor(maxHealth * (100 - 25 * stage) / 100) then
        creature:InterruptNonMeleeSpells(false)
        creature:AttackStop()
        creature:Talk(SAY_SUMMON)
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_SUMMON_SHADE_1, true)
            creature:CastSpell(victim, SPELL_SUMMON_SHADE_2, true)
            creature:CastSpell(victim, SPELL_SUMMON_SHADE_3, true)
        end
        taerarShades[guid] = 3
        taerarBanished[guid] = true
        creature:CastSpell(creature, SPELL_SHADE)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_NON_ATTACKABLE)
        creature:SetReactState(REACT_PASSIVE)
        stages[guid] = stage + 1
        schedule(guid, "taerar_banish", 60000, function()
            taerarEndBanish(creature, guid)
        end)
    end
end

-- C++ boss_taerarAI::SummonedCreatureDies (:613-615): --_shades; the
-- UpdateAI banish branch exits when !_shades. Taerar summons nothing
-- else in the ported model, so any summoned-creature death while
-- banished counts; the 60s timer is the guaranteed exit regardless.
local function taerarSummonedDies(event, creature, summon, killer)
    local guid = creature:GetGUIDLow()
    if not taerarBanished[guid] then
        return
    end
    local shades = (taerarShades[guid] or 0) - 1
    taerarShades[guid] = shades
    if shades <= 0 then
        taerarEndBanish(creature, guid)
    end
end

-- C++ boss_ysondre/boss_lethon/boss_emeriss/boss_taerar::JustEngagedWith:
-- Talk(SAY_*_AGGRO); WorldBossAI::JustEngagedWith(who) — the auriaya
-- precedent (the WorldBossAI legs have no bridges). Each dragon seeds
-- its own timer arm(s) below (the Oct-12 "no-timer bridge" claim was
-- wrong — CreateLuaEvent is a real bridge, retracted this run).
local function ysondreEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "summon_player", 1000, function()
        summonPlayerTick(creature, guid)
    end)
    schedule(guid, "tail_sweep", 4000, function()
        tailSweepTick(creature, guid)
    end)
    schedule(guid, "noxious_breath", {7500, 15000}, function()
        noxiousBreathTick(creature, guid)
    end)
    schedule(guid, "seeping_fog", {12500, 20000}, function()
        seepingFogTick(creature, guid)
    end)
    schedule(guid, "lightning_wave", 12000, function()
        lightningWaveTick(creature, guid)
    end)
end

local function lethonEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "summon_player", 1000, function()
        summonPlayerTick(creature, guid)
    end)
    schedule(guid, "tail_sweep", 4000, function()
        tailSweepTick(creature, guid)
    end)
    schedule(guid, "noxious_breath", {7500, 15000}, function()
        noxiousBreathTick(creature, guid)
    end)
    schedule(guid, "seeping_fog", {12500, 20000}, function()
        seepingFogTick(creature, guid)
    end)
    schedule(guid, "shadow_bolt_whirl", 10000, function()
        shadowBoltWhirlTick(creature, guid)
    end)
end

local function emerissEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "summon_player", 1000, function()
        summonPlayerTick(creature, guid)
    end)
    schedule(guid, "tail_sweep", 4000, function()
        tailSweepTick(creature, guid)
    end)
    schedule(guid, "noxious_breath", {7500, 15000}, function()
        noxiousBreathTick(creature, guid)
    end)
    schedule(guid, "seeping_fog", {12500, 20000}, function()
        seepingFogTick(creature, guid)
    end)
    schedule(guid, "volatile_infection", 12000, function()
        volatileInfectionTick(creature, guid)
    end)
end

local function taerarEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "summon_player", 1000, function()
        summonPlayerTick(creature, guid)
    end)
    schedule(guid, "tail_sweep", 4000, function()
        tailSweepTick(creature, guid)
    end)
    schedule(guid, "noxious_breath", {7500, 15000}, function()
        noxiousBreathTick(creature, guid)
    end)
    schedule(guid, "seeping_fog", {12500, 20000}, function()
        seepingFogTick(creature, guid)
    end)
    schedule(guid, "arcane_blast", 12000, function()
        arcaneBlastTick(creature, guid)
    end)
    schedule(guid, "bellowing_roar", 30000, function()
        bellowingRoarTick(creature, guid)
    end)
end

-- C++ emerald_dragonAI::KilledUnit (:109-113): player-gated
-- who->CastSpell(who, SPELL_MARK_OF_NATURE 25040, triggered). The C++ caster
-- is the victim self-cast; lowered to a triggered creature cast on the
-- victim (maiden_of_virtue / moroes triggered-cast convention) — the aura
-- lands on the same target. Mechanically inert in Go (no spell_mark_of_
-- nature effect), like the ported DamageTaken visual casts in this file.
local function dragonKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:CastSpell(victim, SPELL_MARK_OF_NATURE, true)
    end
end

-- C++ boss_emeriss::KilledUnit (:503-507): DoCast(who, SPELL_PUTRID_
-- MUSHROOM 24904, triggered) on the player victim, then the base
-- emerald_dragonAI::KilledUnit leg above.
local function emerissKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:CastSpell(victim, SPELL_PUTRID_MUSHROOM, true)
        creature:CastSpell(victim, SPELL_MARK_OF_NATURE, true)
    end
end

local function dragonLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    taerarBanished[guid] = nil
    taerarShades[guid] = nil
end

local function dragonDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
end

local function dragonReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    stages[guid] = 1 -- C++ Initialize()/Reset() sets _stage = 1
    -- C++ emerald_dragonAI::Reset (boss_emerald_dragons.cpp:100):
    -- RemoveFlag(UNIT_FIELD_FLAGS, NOT_SELECTABLE|NON_ATTACKABLE)
    -- (ported 2026-10-20); the banish-machine SetFlag pair (:638) is
    -- PORTED in taerarDamageTaken below, so this Reset leg only clears
    -- the base flags.
    -- C++ :101 SetReactState(REACT_AGGRESSIVE) — PORTED: the banish
    -- machine's REACT_PASSIVE :639 can still be live at Reset (evade
    -- mid-banish), and the engine restores no react state on its own;
    -- without this leg taerar stays PASSIVE into the next engage.
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_NON_ATTACKABLE)
    creature:SetReactState(REACT_AGGRESSIVE)
    -- C++ emerald_dragonAI::Reset (:102): DoCast(me, SPELL_MARK_OF_NATURE_
    -- AURA 25041, triggered) — ported for C++-exactness; mechanically
    -- inert in Go (its 10s tick triggers 25042 via the SpellScript-only
    -- spell_mark_of_nature), like the ported KilledUnit 25040 cast.
    creature:CastSpell(creature, SPELL_MARK_OF_NATURE_AURA, true)
end

-- C++ boss_taerarAI::Reset (boss_emerald_dragons.cpp:596-600):
-- RemoveAurasDueToSpell(SPELL_SHADE) then Initialize(), then it DOES call
-- the base emerald_dragonAI::Reset (C++ :602) — the apply leg (DoCast 24313 in the
-- unported 25%-stage banish machine) never fires in Lua, so the removal
-- is currently a no-op — kept C++-exact; the base Reset's RemoveFlag
-- (NOT_SELECTABLE/NON_ATTACKABLE) legs are PORTED in dragonReset
-- above (2026-10-20).
local function taerarReset(event, creature)
    dragonReset(event, creature)
    taerarBanished[creature:GetGUIDLow()] = nil
    taerarShades[creature:GetGUIDLow()] = nil
    creature:RemoveAura(SPELL_SHADE)
end

RegisterCreatureEvent(ENTRY_YSONDRE, 1, ysondreEnterCombat)
RegisterCreatureEvent(ENTRY_YSONDRE, 2, dragonLeaveCombat)
RegisterCreatureEvent(ENTRY_YSONDRE, 4, dragonDied)
RegisterCreatureEvent(ENTRY_YSONDRE, 3, dragonKilledUnit)
RegisterCreatureEvent(ENTRY_YSONDRE, 9, ysondreDamageTaken)
RegisterCreatureEvent(ENTRY_YSONDRE, 23, dragonReset)
RegisterCreatureEvent(ENTRY_LETHON,  1, lethonEnterCombat)
RegisterCreatureEvent(ENTRY_LETHON,  2, dragonLeaveCombat)
RegisterCreatureEvent(ENTRY_LETHON,  4, dragonDied)
RegisterCreatureEvent(ENTRY_LETHON,  3, dragonKilledUnit)
RegisterCreatureEvent(ENTRY_LETHON,  9, lethonDamageTaken)
RegisterCreatureEvent(ENTRY_LETHON,  15, lethonSpellHitTarget)
RegisterCreatureEvent(ENTRY_LETHON,  23, dragonReset)
RegisterCreatureEvent(ENTRY_SPIRIT_SHADE, 22, spiritShadeSummoned)
RegisterCreatureEvent(ENTRY_SPIRIT_SHADE, 28, spiritShadeMovementInform)
RegisterCreatureEvent(ENTRY_SPIRIT_SHADE, 2, spiritShadeCleanup)
RegisterCreatureEvent(ENTRY_SPIRIT_SHADE, 4, spiritShadeCleanup)
RegisterCreatureEvent(ENTRY_EMERISS, 1, emerissEnterCombat)
RegisterCreatureEvent(ENTRY_EMERISS, 2, dragonLeaveCombat)
RegisterCreatureEvent(ENTRY_EMERISS, 4, dragonDied)
RegisterCreatureEvent(ENTRY_EMERISS, 3, emerissKilledUnit)
RegisterCreatureEvent(ENTRY_EMERISS, 9, emerissDamageTaken)
RegisterCreatureEvent(ENTRY_EMERISS, 23, dragonReset)
RegisterCreatureEvent(ENTRY_TAERAR,  1, taerarEnterCombat)
RegisterCreatureEvent(ENTRY_TAERAR,  2, dragonLeaveCombat)
RegisterCreatureEvent(ENTRY_TAERAR,  4, dragonDied)
RegisterCreatureEvent(ENTRY_TAERAR,  3, dragonKilledUnit)
RegisterCreatureEvent(ENTRY_TAERAR,  9, taerarDamageTaken)
RegisterCreatureEvent(ENTRY_TAERAR,  21, taerarSummonedDies)
RegisterCreatureEvent(ENTRY_TAERAR,  23, taerarReset)
