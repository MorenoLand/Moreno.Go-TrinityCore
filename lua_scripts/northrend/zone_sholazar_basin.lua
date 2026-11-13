-- Sholazar Basin zone spell scripts -- Lua ports of the SpellScript
-- arms in src/server/scripts/Northrend/zone_sholazar_basin.cpp.
--
-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_q12611_deathbolt::HandleScriptEffect
--   (zone_sholazar_basin.cpp:658-689): EFFECT_0 SCRIPT_EFFECT. The
--   hit unit carrying Freya's Ward (51845) casts Sevenfold
--   Retribution (51856) on the caster, triggered; otherwise the
--   caster casts Deathbolt (51855) on the hit unit, triggered.
--   Binding 51854 (AC 'spell_q12611_deathbolt'; TDB 335.26091
--   renames it 'spell_sholazar_deathbolt').
local SPELL_DEATHBOLT = 51854 -- zone_sholazar_basin.cpp:658
local SPELL_FREYAS_WARD = 51845 -- zone_sholazar_basin.cpp:654
local SPELL_SEVENFOLD_RETRIBUTION = 51856 -- zone_sholazar_basin.cpp:656
local SPELL_DEATHBOLT_TRIGGER = 51855 -- zone_sholazar_basin.cpp:657

local function deathboltHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if hitTarget:HasAura(SPELL_FREYAS_WARD) then
        hitTarget:CastSpell(caster, SPELL_SEVENFOLD_RETRIBUTION, true)
    else
        caster:CastSpell(hitTarget, SPELL_DEATHBOLT_TRIGGER, true)
    end
end

RegisterSpellEvent(SPELL_DEATHBOLT, 5, deathboltHit)

local SPELL_SHOOT_RJR = 51330 -- zone_sholazar_basin.cpp:460 (class spell_q12589_shoot_rjr)
local NPC_LUCKY_WILHELM = 28054 -- zone_sholazar_basin.cpp:440
local SPELL_FAILED_CUSTOM_ERROR = 172 -- SharedDefines.h:1154
local SPELL_CUSTOM_ERROR_MUST_TARGET_WILHELM = 27 -- SharedDefines.h:1203
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171 (custom value, never sent to the client)

-- C++ spell_q12589_shoot_rjr::CheckCast (zone_sholazar_basin.cpp:470-478):
-- the explicit target must be Lucky Wilhelm (28054) or the cast fails
-- SPELL_FAILED_CUSTOM_ERROR with MUST_TARGET_WILHELM. RegisterSpellEvent
-- OnCheckCast = 8 (lua_spell.go fireSpellCheckCast); the second return
-- is the custom-error id the fire site sends with the failure. Binding
-- 51330 is the AC spell_script_names row. The HandleDummy roll arm
-- (EVENT_MISS/HIT/MISS_BIRD) is PORTED below (shootRjrDummy, ev5).
local function shootRjrCheckCast(event, spell)
    local target = spell:GetTarget()
    if target ~= nil and target:GetEntry() == NPC_LUCKY_WILHELM then
        return SPELL_CAST_OK
    end
    return SPELL_FAILED_CUSTOM_ERROR, SPELL_CUSTOM_ERROR_MUST_TARGET_WILHELM
end

RegisterSpellEvent(SPELL_SHOOT_RJR, 8, shootRjrCheckCast)

-- C++ spell_q12589_shoot_rjr::HandleDummy (zone_sholazar_basin.cpp:480-536):
-- OnEffectHitTarget EFFECT_0 SPELL_EFFECT_DUMMY. urand(1,100): <=50 MISS,
-- <=83 HIT, else MISS_BIRD. MISS: shooter casts 51332 on Wilhelm, Wilhelm
-- Talk(0), Drostan Talk(0). HIT: shooter casts 51331 on the apple, apple
-- self-casts 51371, Wilhelm Talk(1), player KilledMonsterCredit(28053).
-- MISS_BIRD: shooter casts 51366 on Thickbird, bird self-casts 51369,
-- Wilhelm + Drostan Talk(0), bird KillSelf, Crunchy MovePoint(0, bird
-- X/Y, water-or-ground level) — and if bird or crunchy is missing, falls
-- through to the MISS leg ([[fallthrough]], :523-524). Returns silently
-- if Wilhelm, apple, or Drostan is missing (:498-499). All C++ casts are
-- non-triggered (no `true` arg). DORMANT until Spell.dbc ships 51330.
local NPC_APPLE = 28053 -- zone_sholazar_basin.cpp:441
local NPC_DROSTAN = 28328 -- zone_sholazar_basin.cpp:442
local NPC_CRUNCHY = 28346 -- zone_sholazar_basin.cpp:443
local NPC_THICKBIRD = 28093 -- zone_sholazar_basin.cpp:444
local SPELL_HIT_APPLE = 51331 -- zone_sholazar_basin.cpp:447
local SPELL_MISS_APPLE = 51332 -- zone_sholazar_basin.cpp:448
local SPELL_MISS_BIRD_APPLE = 51366 -- zone_sholazar_basin.cpp:449
local SPELL_APPLE_FALL = 51371 -- zone_sholazar_basin.cpp:450
local SPELL_BIRD_FALL = 51369 -- zone_sholazar_basin.cpp:451
local SAY_WILHELM_MISS = 0 -- zone_sholazar_basin.cpp:457
local SAY_WILHELM_HIT = 1 -- zone_sholazar_basin.cpp:458
local SAY_DROSTAN_REPLY_MISS = 0 -- zone_sholazar_basin.cpp:459

local function shootRjrDummy(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local shooter = spell:GetCaster()
    if shooter == nil or hitTarget == nil then
        return
    end
    -- CheckCast guarantees the hit unit is Lucky Wilhelm (28054);
    -- the Creature gate is the C++ GetHitUnit()->ToCreature() twin.
    local wilhelm = hitTarget
    if wilhelm:GetObjectType() ~= "Creature" then
        return
    end
    local apple = shooter:FindNearestCreature(NPC_APPLE, 30.0)
    local drostan = shooter:FindNearestCreature(NPC_DROSTAN, 30.0)
    if apple == nil or drostan == nil then
        return
    end
    local roll = math.random(1, 100)
    if roll <= 50 then
        -- EVENT_MISS
        shooter:CastSpell(wilhelm, SPELL_MISS_APPLE)
        wilhelm:Talk(SAY_WILHELM_MISS)
        drostan:Talk(SAY_DROSTAN_REPLY_MISS)
    elseif roll <= 83 then
        -- EVENT_HIT
        shooter:CastSpell(apple, SPELL_HIT_APPLE)
        apple:CastSpell(apple, SPELL_APPLE_FALL)
        wilhelm:Talk(SAY_WILHELM_HIT)
        local player = shooter:ToPlayer()
        if player ~= nil then
            player:KilledMonsterCredit(NPC_APPLE)
        end
    else
        -- EVENT_MISS_BIRD; falls through to the MISS leg if bird or
        -- crunchy is missing (C++ [[fallthrough]], :523-524).
        local bird = shooter:FindNearestCreature(NPC_THICKBIRD, 30.0)
        local crunchy = shooter:FindNearestCreature(NPC_CRUNCHY, 30.0)
        if bird ~= nil and crunchy ~= nil then
            -- Bird position is read before Kill() so the MovePoint
            -- target survives the kill.
            local bx, by, bz = bird:GetX(), bird:GetY(), bird:GetZ()
            shooter:CastSpell(bird, SPELL_MISS_BIRD_APPLE)
            bird:CastSpell(bird, SPELL_BIRD_FALL)
            wilhelm:Talk(SAY_WILHELM_MISS)
            drostan:Talk(SAY_DROSTAN_REPLY_MISS)
            bird:Kill()
            -- C++ passes GetWaterOrGroundLevel(...) as the MovePoint Z;
            -- no bridge exists (zero hits in engine/), so bird:GetZ()
            -- is used instead.
            crunchy:MoveTo(0, bx, by, bz)
        else
            shooter:CastSpell(wilhelm, SPELL_MISS_APPLE)
            wilhelm:Talk(SAY_WILHELM_MISS)
            drostan:Talk(SAY_DROSTAN_REPLY_MISS)
        end
    end
end

RegisterSpellEvent(SPELL_SHOOT_RJR, 5, shootRjrDummy)

-- C++ spell_shango_tracks::HandleScript (zone_sholazar_basin.cpp:622-636):
-- OnEffectHitTarget EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT; the hit unit
-- Says the correct/incorrect tracks text by spell id. No cast site of
-- 52160/52163 exists in the C++ tree, so the hit unit is assumed to be
-- a creature — if it is the player, the Say leg has no text-id bridge
-- (player Say takes a string) and the handler returns silently. The
-- C++ Say target arg is range/audience, not a whisper, so no whisper
-- target is passed. DORMANT until Spell.dbc ships 52160/52163.
local SPELL_CORRECT_TRACKS = 52160 -- zone_sholazar_basin.cpp:606
local SPELL_INCORRECT_TRACKS = 52163 -- zone_sholazar_basin.cpp:607
local SAY_CORRECT_TRACKS = 28634 -- zone_sholazar_basin.cpp:608
local SAY_INCORRECT_TRACKS = 28635 -- zone_sholazar_basin.cpp:609

local function shangoTracks(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    local id = spell:GetEntry()
    if id == SPELL_CORRECT_TRACKS then
        hitTarget:Talk(SAY_CORRECT_TRACKS)
    elseif id == SPELL_INCORRECT_TRACKS then
        hitTarget:Talk(SAY_INCORRECT_TRACKS)
    end
end

RegisterSpellEvent(SPELL_CORRECT_TRACKS, 5, shangoTracks)
RegisterSpellEvent(SPELL_INCORRECT_TRACKS, 5, shangoTracks)

-- C++ npc_jungle_punch_targetAI::SpellHit (zone_sholazar_basin.cpp:307-335):
-- OnHitBySpell = 14. SPELL_OFFER 51962 only, caster must be a player
-- with QUEST_TASTE_TEST 12645 INCOMPLETE whose objective count for this
-- entry is still 0 (player:GetReqKillOrCastCurrentCount is the
-- CreatureOrGOCount/RequiredNpcOrGo twin, lua_player.go:5013-5037): then
-- KilledMonsterCredit(entry), the player Says the offer line, and the
-- UpdateAI say loop starts — Talk(0..2) at 3.5s then every 3s
-- (:288-306; sayTimer 3500 from Reset, 3000 between lines, stops after
-- the third line). The MoveInLineOfSight orphan phase machine (:229-281)
-- stays out: ev27 is engage-only, and the phase-6 MoveFollow leg has no
-- bridge. Entries DB-verified in creature_template.sql (ScriptName
-- npc_jungle_punch_target): Hemet Nesingwary 27986, Hadrius Harlowe
-- 28047, Tamara Wobblesprocket 28568 — no C++ NPC_ constant exists for
-- them (the C++ reads me->GetEntry() against the quest template).
local SPELL_OFFER = 51962 -- zone_sholazar_basin.cpp:166
local QUEST_TASTE_TEST = 12645 -- zone_sholazar_basin.cpp:167
local QUEST_STATUS_INCOMPLETE = 3 -- quests.go:19
local LANG_UNIVERSAL = 0 -- SharedDefines.h:768
local SAY_OFFER = "Care to try Grimbooze Thunderbrew's new jungle punch?" -- zone_sholazar_basin.cpp:164
local SAY_JUNGLE_PUNCH_1 = 0 -- zone_sholazar_basin.cpp:169
local SAY_JUNGLE_PUNCH_2 = 1 -- zone_sholazar_basin.cpp:170
local SAY_JUNGLE_PUNCH_3 = 2 -- zone_sholazar_basin.cpp:171
local ENTRY_HEMET_NESINGWARY = 27986
local ENTRY_HADRIUS_HARLOWE = 28047
local ENTRY_TAMARA_WOBBLESPROCKET = 28568

local junglePunchTimers = {}

local function junglePunchCancelTimers(guid)
    local per = junglePunchTimers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        junglePunchTimers[guid] = nil
    end
end

local function junglePunchSchedule(guid, key, delay, fn)
    local per = junglePunchTimers[guid]
    if not per then
        per = {}
        junglePunchTimers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- Defined tail-first so no forward declaration is needed.
local function junglePunchSay3(creature, guid)
    creature:Talk(SAY_JUNGLE_PUNCH_3)
    junglePunchTimers[guid] = nil
end

local function junglePunchSay2(creature, guid)
    creature:Talk(SAY_JUNGLE_PUNCH_2)
    junglePunchSchedule(guid, "say3", 3000, function()
        junglePunchSay3(creature, guid)
    end)
end

local function junglePunchSay1(creature, guid)
    creature:Talk(SAY_JUNGLE_PUNCH_1)
    junglePunchSchedule(guid, "say2", 3000, function()
        junglePunchSay2(creature, guid)
    end)
end

local function junglePunchSpellHit(_, creature, caster, spellId)
    if spellId ~= SPELL_OFFER or caster == nil then
        return
    end
    local player = caster:ToPlayer()
    if player == nil then
        return
    end
    if player:GetQuestStatus(QUEST_TASTE_TEST) ~= QUEST_STATUS_INCOMPLETE then
        return
    end
    local entry = creature:GetEntry()
    if player:GetReqKillOrCastCurrentCount(QUEST_TASTE_TEST, entry) ~= 0 then
        return
    end
    player:KilledMonsterCredit(entry)
    player:Say(SAY_OFFER, LANG_UNIVERSAL)
    local guid = creature:GetGUIDLow()
    junglePunchSchedule(guid, "say1", 3500, function()
        junglePunchSay1(creature, guid)
    end)
end

-- C++ Reset (:213-221) re-inits sayStep to 0, which stops the UpdateAI
-- say loop; cancel any pending say timers with it.
local function junglePunchReset(_, creature)
    junglePunchCancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_HEMET_NESINGWARY, 14, junglePunchSpellHit)
RegisterCreatureEvent(ENTRY_HADRIUS_HARLOWE, 14, junglePunchSpellHit)
RegisterCreatureEvent(ENTRY_TAMARA_WOBBLESPROCKET, 14, junglePunchSpellHit)
RegisterCreatureEvent(ENTRY_HEMET_NESINGWARY, 23, junglePunchReset)
RegisterCreatureEvent(ENTRY_HADRIUS_HARLOWE, 23, junglePunchReset)
RegisterCreatureEvent(ENTRY_TAMARA_WOBBLESPROCKET, 23, junglePunchReset)

-- C++ npc_engineer_heliceAI::JustDied (zone_sholazar_basin.cpp:116-122):
-- OnDied = 4. If the escort is running, the escort player fails
-- QUEST_DISASTER 12688. IsEscorted/GetEscortPlayer/FailQuest are all
-- bridged (lua_creature_events.go:2639/2678, lua_player.go:5237).
-- Dormant until the escort machine lands: OnQuestAccept/StartEscort and
-- the WaypointReached chain (:65-113, :140-154) are unported, so
-- IsEscorted never reads true live. Entry DB-verified in
-- creature_template.sql (ScriptName npc_engineer_helice): 28787 — no
-- C++ NPC_ constant exists for it.
local ENTRY_ENGINEER_HELICE = 28787
local QUEST_DISASTER = 12688 -- zone_sholazar_basin.cpp:50

local function engineerHeliceDied(_, creature)
    if not creature:IsEscorted() then
        return
    end
    local player = creature:GetEscortPlayer()
    if player ~= nil then
        player:FailQuest(QUEST_DISASTER)
    end
end

RegisterCreatureEvent(ENTRY_ENGINEER_HELICE, 4, engineerHeliceDied)
