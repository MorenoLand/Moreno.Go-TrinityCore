-- Trollgore (Drak'Tharon Keep) — Lua port of
-- src/server/scripts/Northrend/DraktharonKeep/boss_trollgore.cpp
-- (boss_trollgore (BossAI), npc_drakkari_invader (ScriptedAI),
-- spell_trollgore_consume (SpellScript 49380/59803),
-- spell_trollgore_corpse_explode (AuraScript 49555/59807),
-- spell_trollgore_invader_taunt (SpellScript 49405),
-- achievement_consumption_junction (AchievementCriteriaScript);
-- AddSC_boss_trollgore at end registers all via GetDrakTharonKeepAI /
-- RegisterSpellScript / new). The first Drak'Tharon Keep group in
-- northrend_script_loader.cpp order (decl 39 / call 234, immediately
-- after AddSC_instance_azjol_nerub(); next: AddSC_boss_novos()).
-- Entry: 26630 Trollgore (drak_tharon_keep.h NPC_TROLLGORE —
-- kalecgos pass; the GetDrakTharonKeepAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "boss_trollgore",
-- "npc_drakkari_invader", "spell_trollgore_consume",
-- "spell_trollgore_corpse_explode", "spell_trollgore_invader_taunt"
-- and "achievement_consumption_junction" hits boss_trollgore.cpp only
-- (loader carries only the decl/call lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim (moroes precedent). OnLeaveCombat(2)/OnDied(4)/
-- OnReset(23) cancel the scheduler (gargolmar precedent); BossAI
-- _Reset/_JustDied instance bookkeeping has no bridge.
-- Ported arms (C++-exact for the modeled arms — the two
-- JustEngagedWith schedules are independent, none of the unmodeled
-- events gate them):
-- boss_trollgore: JustEngagedWith Talk(SAY_AGGRO 0) (event 1 — the
-- hook itself is bridged; all five engage-scheduled events are
-- independent of the unmodeled legs);
-- EVENT_CONSUME Talk(SAY_CONSUME 2) + DoCastAOE(Consume 49380),
-- self-cast, 15s init / 15s repeat — the consume-buff stacking is
-- SpellScript-delivered (no bridge); yell + cast cadence ported
-- C++-exact, buff leg stays out (documented-only below);
-- EVENT_CORPSE_EXPLODE Talk(SAY_EXPLODE 3) + DoCastAOE(Corpse Explode
-- 49555), self-cast, 3s init / 15-19s repeat — damage (49618) +
-- despawn are AuraScript-delivered (stale "no bridge" wording:
-- creature:Despawn() IS bridged, but delivery is the unbridged
-- AuraScript AfterEffectRemove machine on a script-only bearer —
-- razelikh precedent); yell + cast cadence
-- ported C++-exact, damage leg stays out (documented-only below);
-- EVENT_SPAWN 30-40s (invader-summoner trigger casts) — PORTED tick 510
-- (see spawnTick below): the three summoners ride the instanceData
-- latch (instance_drak_tharon_keep.lua); the original-caster GUID
-- (Trollgore) has no Lua arm, so each trigger casts as its own caster
-- (CastCustomSpell's originalCaster arg is accepted-and-ignored in Go);
-- the invader landing AI (MovementInform -> dismount/taunt) stays out
-- (motion-master bridge absent).
-- EVENT_CRUSH DoCastVictim(Crush 49639), randtime(1s,5s) init,
-- 10-15s repeat (math.random — slad_ran precedent);
-- EVENT_INFECTED_WOUND DoCastVictim(Infected Wound 49637),
-- randtime(10s,60s) init, 25-35s repeat;
-- KilledUnit Talk(SAY_KILL 1) player-gated (event 3,
-- victim:GetObjectType()=="Player" — nalorakk precedent);
-- JustDied Talk(SAY_DEATH 4) (event 4; _JustDied instance
-- bookkeeping has no bridge).
-- The _consumptionJunction latch — Initialize/Reset true, UpdateAI
-- flips false when the Consume-buff stack exceeds 9 — PORTED tick 418
-- as a SetData mirror (see consumptionJunctionTick below): the Go
-- achievement gate reads the motion's SetData store (bossLatchData),
-- so the old "write-only / dead code" verdict is superseded.
-- Unmodeled (no bridges — documented, not wired):
-- The consume-buff leg of EVENT_CONSUME (SPELL_CONSUME_BUFF 49381
-- stacking via spell_trollgore_consume SpellScript) and the
-- damage+despawn leg of EVENT_CORPSE_EXPLODE (49618 via the
-- spell_trollgore_corpse_explode AuraScript) — no SpellScript/
-- AuraScript binding bridge (razelikh precedent); the yells + cast
-- cadence are ported, these effect legs stay out.
-- The _consumptionJunction latch (Initialize/Reset true; UpdateAI
-- flips it false when GetAura(SPELL_CONSUME_BUFF_HELPER 49381 /
-- 59805)->GetStackAmount() > 9) — PORTED tick 418 as a SetData mirror
-- (see consumptionJunctionTick below): the old "write-only / dead
-- code" verdict is superseded by the Go achievement dispatch, which
-- reads the Lua SetData store via bossLatchData
-- (achievement_criteria_scripts.go) instead of a cross-AI GetData
-- call. Known delta (bronjahm class): the Consume buff itself is
-- applied by the unported spell_trollgore_consume SpellScript, so if
-- Go never stacks the buff the poll never flips and the gate
-- over-awards vs C++ on slow kills.
-- GetData(DATA_CONSUMPTION_JUNCTION 1) — no cross-AI GetData bridge;
-- its only caller is achievement_consumption_junction, whose Go case
-- reads the Lua SetData store instead — see below.
-- JustSummoned — summon->GetMotionMaster()->MovePoint(POINT_LANDING,
-- Landing) + summons.Summon — motion-master bridge absent (summon
-- STRAND absent — standing).
-- UpdateAI HasUnitState(UNIT_STATE_CASTING) pre-loop + in-loop gates
-- PORTED: all 4 ported timer arms defer 500ms while casting (no
-- repeat re-arm — the event stays queued in C++); KilledUnit/JustDied
-- Talk ungated per C++ (outside UpdateAI); the post-loop
-- _consumptionJunction latch is PORTED tick 418 (1s poll, see
-- consumptionJunctionTick below) — the old "stays out (write-only)"
-- verdict is superseded by the Go bossLatchData dispatch.
-- npc_drakkari_invader — ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED (27709 /
-- 27753 / 27754 — drak_tharon_keep.h NPC_DRAKKARI_INVADER_A/B/C,
-- kalecgos pass): zero registration. The whole AI is MovementInform(
-- POINT_MOTION_TYPE, POINT_LANDING) -> me->Dismount() +
-- SetImmuneToAll(false) + DoCastAOE(SPELL_INVADER_TAUNT 49405) —
-- motion-master bridge absent; Dismount bridge live (lua_creature_events.go) —
-- the arm rides the unbridged MovementInform, so it stays out; the
-- SetImmuneToAll(false) leg's flag twin RemoveFlag(59, 0x300) is bridged but
-- never fires without its movement gate (SetImmuneToAll sweep 2026-10-23);
-- the taunt effect is SpellScript-delivered anyway
-- (spell_trollgore_invader_taunt). Joins the
-- entry-verifiable-but-bridge-blocked queue.
-- spell_trollgore_consume (49380/59803 SpellScript: OnEffectHitTarget
-- EFFECT_1 SCRIPT_EFFECT -> target->CastSpell(GetCaster(),
-- SPELL_CONSUME_BUFF 49381, true)) / spell_trollgore_invader_taunt
-- (49405 SpellScript: OnEffectHitTarget SCRIPT_EFFECT ->
-- target->CastSpell(GetCaster(), GetEffectValue(), true)) /
-- spell_trollgore_corpse_explode (49555/59807 AuraScript:
-- OnEffectPeriodic(2nd tick) -> caster->CastSpell(GetTarget(),
-- SPELL_CORPSE_EXPLODE_DAMAGE 49618); AfterEffectRemove -> target
-- ToCreature DespawnOrUnsummon) — no SpellScript/AuraScript binding
-- bridge on the Lua surface (razelikh precedent) — documented-only.
-- achievement_consumption_junction — AchievementCriteriaScript
-- (target ToCreature -> AI()->GetData(DATA_CONSUMPTION_JUNCTION)) —
-- the Go case reads the Lua SetData store via bossLatchData
-- (achievement_criteria_scripts.go); the Lua write leg is ported
-- tick 418 (see consumptionJunctionTick below). No cross-AI GetData
-- bridge is needed for the Go gate.

local ENTRY_TROLLGORE = 26630

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_CONSUME = 2
local SAY_EXPLODE = 3
local SAY_DEATH = 4

local SPELL_CRUSH = 49639
local SPELL_INFECTED_WOUND = 49637
local SPELL_CONSUME = 49380
local SPELL_CORPSE_EXPLODE = 49555
local SPELL_CONSUME_BUFF = 49381 -- boss_trollgore.cpp:34
local SPELL_CONSUME_BUFF_HEROIC = 59805 -- boss_trollgore.cpp:35
local SPELL_SUMMON_INVADER_A = 49456 -- boss_trollgore.cpp:40
local SPELL_SUMMON_INVADER_B = 49457 -- boss_trollgore.cpp:41
local SPELL_SUMMON_INVADER_C = 49458 -- boss_trollgore.cpp:42
local DATA_CONSUMPTION_JUNCTION = 1 -- boss_trollgore.cpp:57

local timers = {}

-- C++ _consumptionJunction latch (boss_trollgore.cpp:84-87, 89-93,
-- 154-159): per-GUID, true until the Consume-buff stack check flips
-- it false; the Go achievement gate reads it from the motion's
-- SetData store (bossLatchData).
local consumptionJunction = {}

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

-- C++ EVENT_CRUSH: DoCastVictim; randtime(1s,5s) init, 10-15s repeat.
-- Own UpdateAI pre-loop + in-loop CASTING gates: while casting the
-- event stays queued — 500ms defer, no repeat re-arm (mal_ganis idiom).
local function crushTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "crush", 500, function()
            crushTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_CRUSH)
    local delay = math.random(10000, 15000)
    schedule(guid, "crush", delay, function()
        crushTick(creature, guid)
    end)
end

-- C++ EVENT_INFECTED_WOUND: DoCastVictim; randtime(10s,60s) init,
-- 25-35s repeat. Same UpdateAI CASTING gates: 500ms defer.
local function infectedWoundTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "infectedwound", 500, function()
            infectedWoundTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_INFECTED_WOUND)
    local delay = math.random(25000, 35000)
    schedule(guid, "infectedwound", delay, function()
        infectedWoundTick(creature, guid)
    end)
end

-- C++ EVENT_CONSUME: Talk(SAY_CONSUME) + DoCastAOE (self-cast); 15s
-- init, 15s repeat. Same UpdateAI CASTING gates: defer precedes
-- Talk/cast (Talk is part of event execution in C++).
local function consumeTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "consume", 500, function()
            consumeTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_CONSUME)
    creature:CastSpell(creature, SPELL_CONSUME)
    schedule(guid, "consume", 15000, function()
        consumeTick(creature, guid)
    end)
end

-- C++ EVENT_CORPSE_EXPLODE: Talk(SAY_EXPLODE) + DoCastAOE (self-cast);
-- 3s init, 15-19s repeat. Same UpdateAI CASTING gates: defer precedes
-- Talk/cast.
local function corpseExplodeTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "corpseexplode", 500, function()
            corpseExplodeTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_EXPLODE)
    creature:CastSpell(creature, SPELL_CORPSE_EXPLODE)
    schedule(guid, "corpseexplode", math.random(15000, 19000), function()
        corpseExplodeTick(creature, guid)
    end)
end

-- C++ UpdateAI post-loop latch (boss_trollgore.cpp:154-159): while
-- _consumptionJunction, if GetAura(SPELL_CONSUME_BUFF_HELPER)->
-- GetStackAmount() > 9, _consumptionJunction = false. The per-tick
-- UpdateAI check rides a 1s re-armed CreateLuaEvent poll seeded on
-- combat start (ichoron frenzy / sindragosa 1s convention); the poll
-- stops re-arming once the latch flips. creature:GetStackAmount
-- (lua_creature_events.go:1493) reads engine-applied auras and
-- returns 0 when absent, subsuming the C++ null-Aura leg.
-- SPELL_CONSUME_BUFF_HELPER is DUNGEON_MODE(49381, 59805)
-- (boss_trollgore.cpp:44); creature:IsHeroic() picks the mode
-- (lua_creature_events.go:3728).
local function consumptionJunctionTick(creature, guid)
    if not consumptionJunction[guid] then
        return
    end
    local buff = creature:IsHeroic() and SPELL_CONSUME_BUFF_HEROIC or SPELL_CONSUME_BUFF
    if creature:GetStackAmount(buff) > 9 then
        consumptionJunction[guid] = false
        creature:SetData(DATA_CONSUMPTION_JUNCTION, 0)
        return
    end
    schedule(guid, "consumptionjunction", 1000, function()
        consumptionJunctionTick(creature, guid)
    end)
end

-- C++ EVENT_SPAWN (boss_trollgore.cpp:143-151): every 30-40s each of
-- the three latched invader summoners (instanceData
-- .trollgoreInvaderSummoners, position-classified world triggers
-- 22515) casts a random summon spell (RAND 49456/49457/49458) on
-- itself. Same UpdateAI CASTING gate as the other arms: 500ms defer,
-- no repeat re-arm. Deviation: the C++ cast carries Trollgore as the
-- original caster (me->GetGUID()); Lua CastSpell has no original-caster
-- arm, so the trigger is its own caster — the summon still spawns the
-- invader at the trigger. The invader landing AI (MovementInform ->
-- dismount/taunt) stays out (motion-master bridge absent). A nil
-- summoners table (summoners not yet latched) skips the cast but still
-- re-arms, exactly like the C++ event re-schedule.
local function spawnTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "spawn", 500, function()
            spawnTick(creature, guid)
        end)
        return
    end
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    local summoners = data and data.trollgoreInvaderSummoners or nil
    if summoners ~= nil then
        local spells = { SPELL_SUMMON_INVADER_A, SPELL_SUMMON_INVADER_B, SPELL_SUMMON_INVADER_C }
        for i = 1, 3 do
            local summoner = summoners[i]
            if summoner ~= nil then
                summoner:CastSpell(summoner, spells[math.random(1, 3)])
            end
        end
    end
    schedule(guid, "spawn", math.random(30000, 40000), function()
        spawnTick(creature, guid)
    end)
end

local function trollgoreEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ Initialize/Reset (boss_trollgore.cpp:84-87/:89-93):
    -- _consumptionJunction starts true; mirror it into the motion's
    -- SetData store for the achievement gate (bossLatchData) and seed
    -- the stack poll.
    consumptionJunction[guid] = true
    creature:SetData(DATA_CONSUMPTION_JUNCTION, 1)
    schedule(guid, "consumptionjunction", 1000, function()
        consumptionJunctionTick(creature, guid)
    end)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    schedule(guid, "crush", math.random(1000, 5000), function()
        crushTick(creature, guid)
    end)
    schedule(guid, "infectedwound", math.random(10000, 60000), function()
        infectedWoundTick(creature, guid)
    end)
    schedule(guid, "consume", 15000, function()
        consumeTick(creature, guid)
    end)
    schedule(guid, "corpseexplode", 3000, function()
        corpseExplodeTick(creature, guid)
    end)
    schedule(guid, "spawn", math.random(30000, 40000), function()
        spawnTick(creature, guid)
    end)
end

local function trollgoreLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function trollgoreTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function trollgoreDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    creature:Talk(SAY_DEATH)
end

local function trollgoreReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    consumptionJunction[guid] = nil
    -- C++ Reset (boss_trollgore.cpp:89-93) re-arms _consumptionJunction
    -- to true. Alive-gated (tick-417 bronjahm/sartharion): ev23 fires
    -- on death too (fireCreatureDied runs Reset before the kill-credit
    -- criteria read, kill.go:1091/:1400), and a death-time re-arm to 1
    -- would clobber a flipped latch before the achievement gate reads it.
    if creature:IsAlive() then
        creature:SetData(DATA_CONSUMPTION_JUNCTION, 1)
    end
end

RegisterCreatureEvent(ENTRY_TROLLGORE, 1, trollgoreEnterCombat)
RegisterCreatureEvent(ENTRY_TROLLGORE, 2, trollgoreLeaveCombat)
RegisterCreatureEvent(ENTRY_TROLLGORE, 3, trollgoreTargetDied)
RegisterCreatureEvent(ENTRY_TROLLGORE, 4, trollgoreDied)
RegisterCreatureEvent(ENTRY_TROLLGORE, 23, trollgoreReset)
