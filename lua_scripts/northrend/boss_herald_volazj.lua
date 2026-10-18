-- Herald Volazj (Ahn'kahet) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/Ahnkahet/boss_herald_volazj.cpp
-- (single script: boss_volazj, BossAI).
-- Ahn'kahet dungeon-script unit per northrend_script_loader.cpp order
-- (decl 36 / call 226, immediately after
-- AddSC_boss_jedoga_shadowseeker(); next: AddSC_instance_ahnkahet()
-- (decl 37 / call 227), which closes the block. FIRST-PASS HEADER
-- DEFECT (re-audited 2026-10-06): the Oct 1 header said "after
-- AddSC_boss_amanitar(); next: boss_jedoga_shadowseeker" — WRONG,
-- the loader order is ...amanitar(34)->jedoga(35)->volazj(36)->
-- instance_ahnkahet(37) (the amanitar-run / jedoga-run precedent).
-- Entry: 29311 Herald Volazj (ahnkahet.h NPC_HERALD_VOLAZJ —
-- kalecgos pass; the RegisterAhnKahetCreatureAI ScriptName binding
-- is instance-shimmed, the creature_template binding DB-side as
-- usual). Sole-source verified: whole-server-tree grep for
-- "boss_volazj" hits boss_herald_volazj.cpp only (loader carries
-- only the decl/call lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell[, triggered]) = DoCastSelf
-- (phase_hunter / apothecary_hanes precedent — the trailing true
-- is the triggered port).
-- Ported arms (C++-exact for all modeled arms):
-- boss_volazj: JustEngagedWith Talk(SAY_AGGRO 0) (event 1);
-- C++ EVENT_MIND_FLAY — DoCastVictim(Mind Flay 57941);
-- JustEngagedWith-scheduled 8s init, rescheduled 20s;
-- C++ EVENT_SHADOW_BOLT_VOLLEY — DoCastVictim(Shadow Bolt Volley
-- 57942); JustEngagedWith-scheduled 5s init, rescheduled 5s
-- (task.Repeat() with no args = same duration — TaskScheduler.h
-- line 485);
-- KilledUnit Talk(SAY_SLAY 1) player-gated (event 3,
-- victim:GetObjectType()=="Player" — nalorakk precedent);
-- JustDied Talk(SAY_DEATH 2) (event 4; _JustDied instance
-- bookkeeping has no bridge).
-- C++ DamageTaken 66%/33% health-crossing -> DoCastSelf(Insanity
-- 57496) (event 9, pre-damage damage arg; InterruptNonMeleeSpells
-- (false) has no bridge — documented; NOT_SELECTABLE damage=0
-- leg needs the absent flag bridge — documented only).
-- Unmodeled (no bridges — documented, not wired):
-- JustEngagedWith _instance->DoStartTimedAchievement(
-- ACHIEVEMENT_TIMED_TYPE_EVENT, ACHIEV_QUICK_DEMISE_START_EVENT
-- 20382) — instance-script model absent (standing blocker); Reset
-- leg _instance->DoStopTimedAchievement — same.
-- EVENT_SHIVER — SelectTarget(SelectTargetMethod::Random, 0) ->
-- DoCast(target, Shiver 57949) (15s repeat): the random-target
-- SelectTarget bridge is absent (cairne/kazzak precedent).
-- DamageTaken: UNIT_FLAG_NOT_SELECTABLE damage-nullification leg
-- (no UNIT_FIELD_FLAGS flag bridge); the 66%/33% crossing arm is
-- PORTED below (event 9; the old "no health-pct bridge (doomwalker
-- precedent)" claim was stale — event 9 carries pre-damage damage
-- and creature:GetHealth()/GetMaxHealth() are bridged);
-- me->InterruptNonMeleeSpells(false) still has no bridge.
-- SpellHitTarget(SPELL_INSANITY 57496) insanity machinery — event
-- 15 never fires (standing); plus the payload needs absent
-- bridges: SummonCreature(NPC_TWISTED_VISAGE 30625,
-- TEMPSUMMON_CORPSE_DESPAWN) (summon STRAND absent — standing),
-- player->CastSpell(summon, Spell Clone Player 57507, triggered)
-- (victim-unit CastSpell bridge absent), summon->SetPhaseMask(
-- 1<<(4+_insanityHandled)) + player phase-mask aura bookkeeping
-- (no phase-mask bridge), me->SetFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NOT_SELECTABLE) + SetControlled(true, UNIT_STATE_
-- STUNNED) (no flag / stun-control bridges), DoCast(me,
-- Insanity Visual 57561, triggered) (bridged in isolation but its
-- trigger is event 15).
-- Reset: SetPhaseMask((1|16|32|64|128|256), true) (no phase-mask
-- bridge); ResetPlayersPhaseMask() is PORTED below via
-- resetPlayersPhaseMask (player:RemoveAura x5 — the old
-- "no aura-removal bridge (aeranas precedent)" claim was stale,
-- player:RemoveAura is bridged, engine/world/lua_player.go:219);
-- RemoveFlag(UNIT_FLAG_NOT_SELECTABLE) + SetControlled(false,
-- UNIT_STATE_STUNNED) (no flag bridges).
-- SummonedCreatureDespawn visage-phase roll-back machine
-- (ObjectAccessor::GetCreature per-summon phase walk, player
-- phasing-aura re-roll 57508-57512): summon STRAND + cross-AI
-- + phase + aura-removal bridges absent.
-- UpdateAI insanity wait-state (UpdateVictim early-return then the
-- _insanityHandled / summons.empty() gate): whole machine rides
-- the unbridged DamageTaken / SpellHitTarget / summon arms.
-- npc_twisted_visage: NO AI EXISTS in C++ (file header comment
-- "Missing AI for Twisted Visages" — zero Register* lines for it);
-- NPC_TWISTED_VISAGE = 30625 is entry-verifiable (ahnkahet.h —
-- kalecgos pass) but there is nothing to port — no registration,
-- documented as missing upstream.
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler
-- (gargolmar precedent); BossAI _Reset/_JustDied instance
-- bookkeeping has no bridge.

local ENTRY_HERALD_VOLAZJ = 29311

local SPELL_MIND_FLAY = 57941
local SPELL_SHADOW_BOLT_VOLLEY = 57942
local SPELL_INSANITY = 57496 -- boss_herald_volazj.cpp:34

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
-- (C++ Yells enum also declares SAY_PHASE = 3, but it is never
-- used in boss_herald_volazj.cpp — the sole usage grep hits only
-- the enum line — so nothing to port.)

local SPELL_INSANITY_PHASING_1 = 57508 -- boss_herald_volazj.cpp:41
local SPELL_INSANITY_PHASING_2 = 57509 -- boss_herald_volazj.cpp:42
local SPELL_INSANITY_PHASING_3 = 57510 -- boss_herald_volazj.cpp:43
local SPELL_INSANITY_PHASING_4 = 57511 -- boss_herald_volazj.cpp:44
local SPELL_INSANITY_PHASING_5 = 57512 -- boss_herald_volazj.cpp:45

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

-- boss_herald_volazj.cpp:173 ResetPlayersPhaseMask. C++ removes
-- GetSpellForPhaseMask(player->GetPhaseMask()) per map player — the
-- one phasing aura matching their phase (16->57508, 32->57509,
-- 64->57510, 128->57511, 256->57512; phase 1 maps to 0, a no-op).
-- player:GetPhaseMask() has no bridge, so the port strips all five
-- phasing auras per same-map/instance player instead: absent auras
-- are no-ops, so the net state is C++-exact. player:RemoveAura is
-- bridged (engine/world/lua_player.go:219); the GetPlayersInWorld +
-- map/instance filter pool is the archavon / anomalus precedent.
local function resetPlayersPhaseMask(creature)
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    local phasing = {
        SPELL_INSANITY_PHASING_1, SPELL_INSANITY_PHASING_2,
        SPELL_INSANITY_PHASING_3, SPELL_INSANITY_PHASING_4,
        SPELL_INSANITY_PHASING_5,
    }
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId then
            for _, spell in ipairs(phasing) do
                plr:RemoveAura(spell)
            end
        end
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

-- C++ EVENT_MIND_FLAY: JustEngagedWith-scheduled 8s init,
-- rescheduled 20s; DoCastVictim. UpdateAI returns early while
-- casting — the Go engine handles cast-gating, the timer just
-- re-arms.
local function mindFlayTick(creature, guid)
    creature:CastSpell(nil, SPELL_MIND_FLAY)
    schedule(guid, "mindflay", 20000, function()
        mindFlayTick(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_BOLT_VOLLEY: JustEngagedWith-scheduled 5s
-- init, task.Repeat() = same 5s duration (TaskScheduler.h line
-- 485); DoCastVictim.
local function shadowBoltTick(creature, guid)
    creature:CastSpell(nil, SPELL_SHADOW_BOLT_VOLLEY)
    schedule(guid, "shadowbolt", 5000, function()
        shadowBoltTick(creature, guid)
    end)
end

local function volazjEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "mindflay", 8000, function()
        mindFlayTick(creature, guid)
    end)
    schedule(guid, "shadowbolt", 5000, function()
        shadowBoltTick(creature, guid)
    end)
end

local function volazjLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function volazjTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function volazjDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
    resetPlayersPhaseMask(creature) -- boss_herald_volazj.cpp:284, C++
                                     -- arm order (after Talk)
end

local function volazjReset(event, creature)
    cancelTimers(creature:GetGUID())
    resetPlayersPhaseMask(creature) -- boss_herald_volazj.cpp:80, C++
                                     -- arm order (after the SetPhaseMask
                                     -- leg, whose bridge is absent)
end

-- C++ DamageTaken (cpp:114-134): GetHealthPct(damage) with
-- damage > health -> 0 and integer division
-- (100*(health-damage)/maxHealth). Downward crossing of 66% or 33%
-- -> me->InterruptNonMeleeSpells(false) (no bridge, documented)
-- + DoCast(me, SPELL_INSANITY, false) = self-cast, non-triggered
-- (vaelastrasz convention: CastSpell(creature, spell) = DoCastSelf).
-- The UNIT_FLAG_NOT_SELECTABLE damage=0 leg needs the absent flag
-- bridge; the flag is only set by the unported SpellHitTarget
-- insanity machine (event 15 never fires), so it never fires in
-- Lua. Event 9 fires pre-damage carrying damage (eck precedent),
-- so pre/post pct are C++-exact. No once-latch in C++: a re-cross
-- after a heal re-fires.
local function volazjDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local function pctAfter(dmg)
        if dmg > health then
            return 0
        end
        return math.floor(100 * (health - dmg) / maxHealth)
    end
    local pre = pctAfter(0)
    local post = pctAfter(damage)
    if (pre >= 66 and post < 66) or (pre >= 33 and post < 33) then
        creature:CastSpell(creature, SPELL_INSANITY)
    end
end

RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 1, volazjEnterCombat)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 2, volazjLeaveCombat)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 3, volazjTargetDied)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 4, volazjDied)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 23, volazjReset)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 9, volazjDamageTaken)
