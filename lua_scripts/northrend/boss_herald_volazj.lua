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
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 15 OnSpellHitTarget, 20
-- OnSummonedCreatureDespawn, 23 OnReset. Timers via CreateLuaEvent;
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
-- C++ EVENT_SHIVER — SelectTarget(Random, 0) -> DoCast(target,
-- Shiver 57949), 15s init, 15s repeat (task.Repeat() outside the
-- target-if — unconditional re-arm): PORTED 2026-10-25 — the old
-- "random-target SelectTarget bridge absent (cairne/kazzak)" claim
-- is stale (GetAITargets is the bridge, tharon_ja precedent);
-- playerOnly defaults false and dist 0.0f = any range, so the pick
-- is a random alive threat-list unit of any type (C++ skips
-- offline refs, UnitAI.h; Go SortedEntries returns dead too, so
-- IsAlive filters). The insanity wait-state pause of the scheduler rides the
-- ported insanity machine (wait-state pump below) — but the BossAI
-- scheduler CASTING validator DOES
-- gate the ported arms (ScriptedCreature.cpp:493-496): mind-flay /
-- bolt / shiver all defer 500ms while casting, no repeat re-arm
-- (see mindFlayTick). DamageTaken / KilledUnit / JustDied stay
-- ungated per C++.
-- KilledUnit Talk(SAY_SLAY 1) player-gated (event 3,
-- victim:GetObjectType()=="Player" — nalorakk precedent);
-- JustDied Talk(SAY_DEATH 2) (event 4; _JustDied instance
-- bookkeeping has no bridge).
-- C++ DamageTaken 66%/33% health-crossing -> DoCastSelf(Insanity
-- 57496) (event 9, pre-damage damage arg; InterruptNonMeleeSpells
-- (false) has no bridge — documented; NOT_SELECTABLE damage=0
-- leg reads creature:HasFlag (bridged, lua_creature_events.go:6374)
-- and is LIVE since the ev15 machine port (2026-11-14): the flag is
-- set by the first insanity hit, cleared by the wait-state pump.
-- Unmodeled (no bridges — documented, not wired):
-- JustEngagedWith _instance->DoStartTimedAchievement(
-- ACHIEVEMENT_TIMED_TYPE_EVENT, ACHIEV_QUICK_DEMISE_START_EVENT
-- 20382) — instance-script model absent (standing blocker); Reset
-- leg _instance->DoStopTimedAchievement — same.
-- EVENT_SHIVER — PORTED above (see ported-arms list); the old
-- "random-target SelectTarget bridge absent" note is retired.
-- DamageTaken: UNIT_FLAG_NOT_SELECTABLE damage-nullification leg
-- (creature:HasFlag bridged, but the flag is never set in the
-- ported model — documented only); the 66%/33% crossing arm is
-- PORTED below (event 9; the old "no health-pct bridge (doomwalker
-- precedent)" claim was stale — event 9 carries pre-damage damage
-- and creature:GetHealth()/GetMaxHealth() are bridged);
-- me->InterruptNonMeleeSpells(false) — PORTED below (no-op twin,
-- curator precedent).
-- SpellHitTarget(SPELL_INSANITY 57496) insanity machinery — PORTED
-- 2026-11-14 (whole machine: ev15 + ev20 + wait-state pump +
-- visage tracking, see below): 57496 seeded from wago 3.4.0.43659
-- (ImplicitTargetA 22 SRC_CASTER / B 15 SRC_AREA_ENEMY, radius 28)
-- so the 66%/33% self-cast hits players and ev15 fires per player
-- (boss_ai.go:4234-4290); the old "no Spell.dbc / spellKnown=false"
-- stay-out is retired. First hit starts the visual (57561),
-- NOT_SELECTABLE + stun; each hit phases its player (57508+handled)
-- and summons a Twisted Visage per alive map player, cloned via
-- player:CastSpell (triggered-only funnel, lua_player.go:3531) and
-- phased 1<<(4+handled) (SetPhaseMask bridged).
-- Reset: SetPhaseMask((1|16|32|64|128|256), true) (no phase-mask
-- bridge); ResetPlayersPhaseMask() is PORTED below via
-- resetPlayersPhaseMask (player:RemoveAura x5 — the old
-- "no aura-removal bridge (aeranas precedent)" claim was stale,
-- player:RemoveAura is bridged, engine/world/lua_player.go:4329);
-- RemoveFlag(UNIT_FLAG_NOT_SELECTABLE) PORTED (2026-10-20);
-- SetControlled(false, UNIT_STATE_STUNNED) PORTED 2026-11-13
-- (follow-up strip 1, unit A — creature:SetStunned(false) in
-- volazjReset below).
-- SummonedCreatureDespawn visage-phase roll-back machine — PORTED
-- 2026-11-14 (ev20, C++ :205-232): tracked visages keep summon
-- order (hyjal precedent); a same-phase survivor blocks the roll,
-- else players holding the despawned phase's aura roll to the last
-- surviving phase (player:HasAura/RemoveAura/CastSpell bridged).
-- UpdateAI insanity wait-state (C++ :262-270) — PORTED 2026-11-14
-- as a 1s CreateLuaEvent pump: while visages remain the scheduler
-- stays paused (rotation ticks poll insanityActive); once empty,
-- handled resets, NOT_SELECTABLE + stun + visual clear. The
-- DamageTaken crossing arm is ported above; ev15/ev20 registered
-- below.
-- npc_twisted_visage: NO AI EXISTS in C++ (file header comment
-- "Missing AI for Twisted Visages" — zero Register* lines for it);
-- NPC_TWISTED_VISAGE = 30625 is entry-verifiable (ahnkahet.h —
-- kalecgos pass) but there is nothing to port — no registration,
-- documented as missing upstream.
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler
-- (gargolmar precedent); BossAI _Reset/_JustDied instance
-- bookkeeping ports in volazjDied below (SetBossState + flag).

local ENTRY_HERALD_VOLAZJ = 29311
local DATA_HERALD_VOLAZJ = 4 -- ahnkahet.h:35
local BOSS_STATE_DONE = 3 -- EncounterState DONE (InstanceScript.h:68-76)

local SPELL_MIND_FLAY = 57941
local SPELL_SHADOW_BOLT_VOLLEY = 57942
local SPELL_SHIVER = 57949
local SPELL_INSANITY = 57496 -- boss_herald_volazj.cpp:34
local SPELL_INSANITY_VISUAL = 57561 -- boss_herald_volazj.cpp:35
local SPELL_CLONE_PLAYER = 57507 -- boss_herald_volazj.cpp:40
local NPC_TWISTED_VISAGE = 30625 -- ahnkahet.h:70
local TEMPSUMMON_CORPSE_DESPAWN = 4 -- Go 0-based TempSummonType scheme (lua_creature_events.go:58; C++ 5)

-- UnitDefines.h:149; RemoveFlag on UNIT_FIELD_FLAGS (59) is bridged
-- (engine/world/lua_creature_events.go:6540).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

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
-- Phase masks for the phasing auras (MiscValue): 57508->16,
-- 57509->32, 57510->64, 57511->128, 57512->256.
local INSANITY_PHASES = { 16, 32, 64, 128, 256 }

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
-- bridged (engine/world/lua_player.go:4329); the GetPlayersInWorld +
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

local insanity = {} -- per-GUID { handled = n, visages = { { guid, phase } } }

local function insanityState(guid)
    local st = insanity[guid]
    if not st then
        st = { handled = 0, visages = {} }
        insanity[guid] = st
    end
    return st
end

-- C++ UpdateAI wait-state gate (cpp:262-264): while insanity is
-- handled the BossAI scheduler never runs — the rotation ticks poll
-- this and re-arm on a 1s delay instead of casting.
local function insanityActive(guid)
    local st = insanity[guid]
    return st ~= nil and st.handled > 0
end

local function spellForPhaseMask(phase)
    if phase == 16 then
        return SPELL_INSANITY_PHASING_1
    elseif phase == 32 then
        return SPELL_INSANITY_PHASING_2
    elseif phase == 64 then
        return SPELL_INSANITY_PHASING_3
    elseif phase == 128 then
        return SPELL_INSANITY_PHASING_4
    elseif phase == 256 then
        return SPELL_INSANITY_PHASING_5
    end
    return 0
end

-- BossAI::_Reset/_JustDied (ScriptedCreature.cpp:499-516):
-- summons.DespawnAll() — each visage despawn fires ev20 below, but
-- the state is cleared first so the roll-back (moot: Reset/Died
-- strip all phasing auras right after) does not run mid-iteration.
local function despawnVisages(creature)
    local guid = creature:GetGUIDLow()
    local st = insanity[guid]
    if not st then
        return
    end
    insanity[guid] = nil
    for _, v in ipairs(st.visages) do
        local visage = creature:GetCreatureByGUID(v.guid)
        if visage then
            visage:Despawn()
        end
    end
end

-- C++ EVENT_MIND_FLAY: JustEngagedWith-scheduled 8s init,
-- rescheduled 20s; DoCastVictim. BossAI scheduler validator
-- (ScriptedCreature.cpp:493-496, set in the BossAI ctor): while
-- casting, Dispatch returns with due tasks still queued — 500ms
-- defer, no repeat re-arm (mal_ganis idiom). The old "Go engine
-- handles cast-gating, the timer just re-arms" claim was wrong:
-- the validator gates these tasks even though volazj overrides
-- UpdateAI, because the scheduler itself is BossAI's.
local function mindFlayTick(creature, guid)
    if insanityActive(guid) then
        schedule(guid, "mindflay", 1000, function()
            mindFlayTick(creature, guid)
        end)
        return
    end
    if creature:IsCasting() then
        schedule(guid, "mindflay", 500, function()
            mindFlayTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_MIND_FLAY)
    schedule(guid, "mindflay", 20000, function()
        mindFlayTick(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_BOLT_VOLLEY: JustEngagedWith-scheduled 5s
-- init, task.Repeat() = same 5s duration (TaskScheduler.h line
-- 485); DoCastVictim.
local function shadowBoltTick(creature, guid)
    if insanityActive(guid) then
        schedule(guid, "shadowbolt", 1000, function()
            shadowBoltTick(creature, guid)
        end)
        return
    end
    if creature:IsCasting() then
        schedule(guid, "shadowbolt", 500, function()
            shadowBoltTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_SHADOW_BOLT_VOLLEY)
    schedule(guid, "shadowbolt", 5000, function()
        shadowBoltTick(creature, guid)
    end)
end

-- C++ EVENT_SHIVER: ScheduleTasks 15s init, task.Repeat() = same
-- 15s, unconditional (outside the target-if); SelectTarget(Random,
-- 0) = random threat-list unit, any type, any range (playerOnly
-- defaults false, dist 0.0f); C++ skips offline refs, so the Lua
-- pool filters IsAlive (UnitAI.h; Go SortedEntries returns dead
-- too). No cast on an empty pool; DoCast non-triggered.
local function shiverTick(creature, guid)
    if insanityActive(guid) then
        schedule(guid, "shiver", 1000, function()
            shiverTick(creature, guid)
        end)
        return
    end
    if creature:IsCasting() then
        schedule(guid, "shiver", 500, function()
            shiverTick(creature, guid)
        end)
        return
    end
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_SHIVER)
    end
    schedule(guid, "shiver", 15000, function()
        shiverTick(creature, guid)
    end)
end

local function volazjEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- BossAI::JustEngagedWith base arms, base-first (C++ arm order);
    -- instance/setActive/achievement legs out.
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    schedule(guid, "mindflay", 8000, function()
        mindFlayTick(creature, guid)
    end)
    schedule(guid, "shadowbolt", 5000, function()
        shadowBoltTick(creature, guid)
    end)
    schedule(guid, "shiver", 15000, function()
        shiverTick(creature, guid)
    end)
end

local function volazjLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function volazjTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function volazjDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    -- C++ BossAI::_JustDied (ScriptedCreature.cpp:513-520):
    -- instance->SetBossState(DATA_HERALD_VOLAZJ, DONE); the flag
    -- rides the same instance-consumer pattern as
    -- instance_ahnkahet.lua (no boss-state-change event exists).
    creature:SetBossState(DATA_HERALD_VOLAZJ, BOSS_STATE_DONE)
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data.heraldVolazjDone = true
    end
    despawnVisages(creature) -- BossAI::_JustDied summons.DespawnAll()
    creature:Talk(SAY_DEATH)
    resetPlayersPhaseMask(creature) -- boss_herald_volazj.cpp:284, C++
                                     -- arm order (after Talk)
end

-- C++ LoadObjectData (instance_ahnkahet.cpp:30-37): latch the boss
-- GUID under DATA_HERALD_VOLAZJ (instance_guid_store.go; the
-- instance-side OnCreatureCreate route is dead for static spawns
-- — instance_ahnkahet.lua header).
local function volazjSpawned(event, creature)
    creature:SetGuidData(DATA_HERALD_VOLAZJ)
end

local function volazjReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    despawnVisages(creature) -- BossAI::_Reset summons.DespawnAll()
    resetPlayersPhaseMask(creature) -- boss_herald_volazj.cpp:80, C++
                                     -- arm order (after the SetPhaseMask
                                     -- leg, whose bridge is absent)
    -- boss_herald_volazj.cpp:83 — C++ arm order after ResetPlayersPhaseMask
    -- (ported 2026-10-20); boss_herald_volazj.cpp:84 SetControlled(false,
    -- UNIT_STATE_STUNNED) PORTED 2026-11-13 (follow-up strip 1, unit A —
    -- creature:SetStunned is bridged, lua_creature_events.go). Live
    -- clear since the ev15 machine port (2026-11-14): the stun is set
    -- by the first insanity hit.
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetStunned(false)
end

-- C++ DamageTaken (cpp:114-134): GetHealthPct(damage) with
-- damage > health -> 0 and integer division
-- (100*(health-damage)/maxHealth). Downward crossing of 66% or 33%
-- -> me->InterruptNonMeleeSpells(false) (ported below, no-op twin)
-- + DoCast(me, SPELL_INSANITY, false) = self-cast, non-triggered
-- (vaelastrasz convention: CastSpell(creature, spell) = DoCastSelf).
-- The UNIT_FLAG_NOT_SELECTABLE damage=0 leg is LIVE since the ev15
-- machine port (2026-11-14): the flag is set by the first insanity
-- hit, and the rewrite rides the ev9 second return (garr precedent:
-- return false, 0). Event 9 fires pre-damage carrying damage (eck
-- precedent),
-- so pre/post pct are C++-exact. No once-latch in C++: a re-cross
-- after a heal re-fires.
local function volazjDamageTaken(event, creature, attacker, damage)
    -- C++ DamageTaken (cpp:114-117): NOT_SELECTABLE nullifies the hit;
    -- with damage 0 the crossing check below cannot fire (post == pre).
    if creature:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE) then
        return false, 0
    end
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
        creature:InterruptNonMeleeSpells(false)
        creature:CastSpell(creature, SPELL_INSANITY)
    end
    return false, damage
end

-- C++ UpdateAI wait-state (cpp:262-270) as a 1s pump: while visages
-- remain, re-arm; once empty, handled resets, NOT_SELECTABLE clears,
-- the stun clears and the visual aura drops. Runs only while insanity
-- is handled (started by the first ev15 hit, cancelled by
-- cancelTimers on ev2/4/23).
local function insanityWaitTick(creature, guid)
    local st = insanity[guid]
    if not st or st.handled == 0 then
        return
    end
    if #st.visages > 0 then
        schedule(guid, "insanity", 1000, function()
            insanityWaitTick(creature, guid)
        end)
        return
    end
    insanity[guid] = nil
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetStunned(false)
    creature:RemoveAura(SPELL_INSANITY_VISUAL)
end

-- C++ SpellHitTarget (cpp:135-171): Insanity hit on a player — up to
-- 5 hits (handled 0-4); first hit starts the channel visual, sets
-- NOT_SELECTABLE + stun, and starts the wait-state pump; every hit
-- phases its player (57508+handled) and summons a Twisted Visage per
-- alive map player, cloned from that player (57507) and phased
-- 1<<(4+handled). The boss itself is a hit target of the self-cast
-- (ImplicitTargetA 22) and is rejected by the player gate.
local function volazjSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_INSANITY then
        return
    end
    if not target or target:GetObjectType() ~= "Player" then
        return
    end
    local guid = creature:GetGUIDLow()
    local st = insanityState(guid)
    if st.handled > 4 then
        return
    end
    if st.handled == 0 then
        creature:CastSpell(creature, SPELL_INSANITY_VISUAL, true)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:SetStunned(true)
        schedule(guid, "insanity", 1000, function()
            insanityWaitTick(creature, guid)
        end)
    end
    target:CastSpell(target, SPELL_INSANITY_PHASING_1 + st.handled)
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    local x, y, z, o = creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO()
    local phase = INSANITY_PHASES[st.handled + 1]
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId and plr:IsAlive() then
            local visage = creature:SummonCreature(NPC_TWISTED_VISAGE, x, y, z, o, TEMPSUMMON_CORPSE_DESPAWN)
            if visage then
                plr:CastSpell(visage, SPELL_CLONE_PLAYER)
                visage:SetPhaseMask(phase, true)
                st.visages[#st.visages + 1] = { guid = visage:GetGUIDLow(), phase = phase }
            end
        end
    end
    st.handled = st.handled + 1
end

-- C++ SummonedCreatureDespawn (cpp:205-232): prune the visage, then
-- if another visage in the same phase is still alive, wait; else roll
-- every player holding this phase's aura to the last surviving phase
-- (or strip it when none survives). The loop keeps summon order, so
-- nextPhase is the last remaining visage's phase, C++-exact.
local function volazjSummonDespawn(event, creature, summon)
    local guid = creature:GetGUIDLow()
    local st = insanity[guid]
    if not st then
        return
    end
    local sguid = summon:GetGUIDLow()
    local phase = 0
    for i, v in ipairs(st.visages) do
        if v.guid == sguid then
            phase = v.phase
            table.remove(st.visages, i)
            break
        end
    end
    if phase == 0 then
        return
    end
    local nextPhase = 0
    for _, v in ipairs(st.visages) do
        if v.phase == phase then
            return
        end
        nextPhase = v.phase
    end
    local spell = spellForPhaseMask(phase)
    local spell2 = spellForPhaseMask(nextPhase)
    if spell == 0 then
        return
    end
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId and plr:HasAura(spell) then
            plr:RemoveAura(spell)
            if spell2 ~= 0 then
                plr:CastSpell(plr, spell2)
            end
        end
    end
end

RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 1, volazjEnterCombat)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 2, volazjLeaveCombat)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 3, volazjTargetDied)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 4, volazjDied)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 5, volazjSpawned)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 23, volazjReset)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 9, volazjDamageTaken)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 15, volazjSpellHitTarget)
RegisterCreatureEvent(ENTRY_HERALD_VOLAZJ, 20, volazjSummonDespawn)
