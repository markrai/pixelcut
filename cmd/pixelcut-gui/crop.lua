local mp = require("mp")
local utils = require("mp.utils")
local overlay = mp.create_osd_overlay("ass-events")
local req, box, drag, action = nil, nil, nil, ""
local editing = false
local ready = false

local function publish(ready, err)
    if not req then return end
    mp.set_property_native("user-data/pixelcut/crop", {
        serial=req.serial, crop=box or req.crop, ready=ready,
        dragging=drag~=nil, error=err or "", action=action
    })
end

local function geometry()
    local d=mp.get_property_native("osd-dimensions")
    if not d or not d.w or not d.h or d.w<=0 or d.h<=0 then return end
    local l,t=d.ml or 0,d.mt or 0
    local w,h=d.w-l-(d.mr or 0),d.h-t-(d.mb or 0)
    if w<=0 or h<=0 then return end
    return {l=l,t=t,w=w,h=h,sw=d.w,sh=d.h}
end

local function even(n) return math.floor(n/2+0.5)*2 end
local function clamp(n,l,h) return math.max(l,math.min(h,n)) end
local function copy(b) return {x=b.x,y=b.y,width=b.width,height=b.height} end

local function rect(l,t,r,b,color,alpha)
    if r<=l or b<=t then return "" end
    return string.format("{\\an7\\pos(0,0)\\bord0\\shad0\\1c&H%s&\\1a&H%s&\\p1}m %.0f %.0f l %.0f %.0f %.0f %.0f %.0f %.0f{\\p0}",color,alpha,l,t,r,t,r,b,l,b)
end

local function redraw()
    if not editing or not box then overlay:remove();return end
    local g=geometry()
    if not g then return end
    local l,t=g.l+box.x*g.w/req.width,g.t+box.y*g.h/req.height
    local r,b=l+box.width*g.w/req.width,t+box.height*g.h/req.height
    local out={}
    local function add(...) out[#out+1]=rect(...) end
    add(g.l,g.t,g.l+g.w,t,"000000","60")
    add(g.l,b,g.l+g.w,g.t+g.h,"000000","60")
    add(g.l,t,l,b,"000000","60")
    add(r,t,g.l+g.w,b,"000000","60")
    local yellow="1BCEFF"
    add(l,t,r,t+2,yellow,"00")
    add(l,b-2,r,b,yellow,"00")
    add(l,t,l+2,b,yellow,"00")
    add(r-2,t,r,b,yellow,"00")
    for i=1,2 do
        local x=l+(r-l)*i/3
        local y=t+(b-t)*i/3
        add(x,t,x+1,b,"FFFFFF","80")
        add(l,y,r,y+1,"FFFFFF","80")
    end
    for _,x in ipairs({l,(l+r)/2,r}) do
        for _,y in ipairs({t,(t+b)/2,b}) do
            if x~=(l+r)/2 or y~=(t+b)/2 then add(x-4,y-4,x+4,y+4,yellow,"00") end
        end
    end
    overlay.res_x=g.sw
    overlay.res_y=g.sh
    overlay.data=table.concat(out,"\n")
    overlay:update()
end

local function show_result()
    local ok,err=mp.set_property("video-crop","")
    if not ok then publish(false,tostring(err));return end
    if type(req.filters)=="table" then
        if req.limited_only and mp.get_property("video-params/colorlevels")~="limited" then
            publish(false,"Canvas transforms require limited-range HLG");return
        end
        local rotation=(mp.get_property_number("video-params/rotate",0)%360+360)%360
        local graph=req.filters[tostring(rotation)]
        if not graph then publish(false,"Unsupported preview rotation");return end
        ok,err=mp.commandv("vf","set",graph)
    elseif box.x~=0 or box.y~=0 or box.width~=req.width or box.height~=req.height then
        publish(false,"Crop filter plan missing");return
    else
        ok,err=mp.commandv("vf","clr","")
    end
    publish(ok==true,ok and "" or tostring(err))
end

local function mouse()
    local m=mp.get_property_native("mouse-pos")
    if not m then return end
    return m.x,m.y
end

local function start_drag(locked)
    if not editing or not ready then return end
    local g=geometry()
    local mx,my=mouse()
    if not g or not mx then return end
    local l,t=g.l+box.x*g.w/req.width,g.t+box.y*g.h/req.height
    local r,b=l+box.width*g.w/req.width,t+box.height*g.h/req.height
    local hx,hy=0,0
    if math.abs(mx-l)<=9 then hx=-1 elseif math.abs(mx-r)<=9 then hx=1 end
    if math.abs(my-t)<=9 then hy=-1 elseif math.abs(my-b)<=9 then hy=1 end
    if mx<l-9 or mx>r+9 or my<t-9 or my>b+9 then return end
    drag={x=mx,y=my,box=copy(box),hx=hx,hy=hy,lock=locked,g=g}
    publish(true)
end

local function move_drag()
    if not drag then return end
    local mx,my=mouse()
    if not mx then return end
    local d=drag
    local o=d.box
    local dx=even((mx-d.x)*req.width/d.g.w)
    local dy=even((my-d.y)*req.height/d.g.h)
    if d.hx==0 and d.hy==0 then
        box={x=clamp(o.x+dx,0,req.width-o.width),y=clamp(o.y+dy,0,req.height-o.height),width=o.width,height=o.height}
    else
        local l,t,r,b=o.x,o.y,o.x+o.width,o.y+o.height
        if d.hx<0 then l=clamp(l+dx,0,r-2) elseif d.hx>0 then r=clamp(r+dx,l+2,req.width) end
        if d.hy<0 then t=clamp(t+dy,0,b-2) elseif d.hy>0 then b=clamp(b+dy,t+2,req.height) end
        if d.lock then
            local ratio=o.width/o.height
            local w,h=r-l,b-t
            local maxw=d.hx<0 and o.x+o.width or req.width-o.x
            local maxh=d.hy<0 and o.y+o.height or req.height-o.y
            if d.hx==0 then w=even(h*ratio) else h=even(w/ratio) end
            if w>maxw then w=maxw;h=even(w/ratio) end
            if h>maxh then h=maxh;w=even(h*ratio) end
            w=clamp(w,2,maxw);h=clamp(h,2,maxh)
            if d.hx<0 then l=o.x+o.width-w;r=o.x+o.width else l=o.x;r=l+w end
            if d.hy<0 then t=o.y+o.height-h;b=o.y+o.height else t=o.y;b=t+h end
        end
        box={x=l,y=t,width=r-l,height=b-t}
    end
    redraw()
    publish(true)
end

local function release()
    if drag then move_drag();drag=nil;publish(true) end
end

local function bind_mouse(key,name,locked)
    mp.add_forced_key_binding(key,name,function(e)
        if e.event=="down" then start_drag(locked)
        elseif e.event=="up" then release() end
    end,{complex=true})
end

local function remove_bindings()
    for _,name in ipairs({"pc-crop-mouse","pc-crop-shift","pc-crop-enter","pc-crop-escape","pc-crop-space"}) do mp.remove_key_binding(name) end
end

mp.register_script_message("pixelcut-crop",function(payload)
    local v=utils.parse_json(payload)
    if not v or not v.crop or not v.width or not v.height then return end
    req=v;box=copy(v.crop);drag=nil;action="";editing=v.edit;ready=false
    remove_bindings()
    if editing then
        mp.set_property_bool("pause",true)
        local ok,err=mp.commandv("vf","clr","")
        if not ok then publish(false,tostring(err));return end
        ok,err=mp.set_property("video-crop","")
        if not ok then publish(false,tostring(err));return end
        bind_mouse("MBTN_LEFT","pc-crop-mouse",false)
        bind_mouse("Shift+MBTN_LEFT","pc-crop-shift",true)
        mp.add_forced_key_binding("ENTER","pc-crop-enter",function() if not drag then action="apply";publish(true) end end)
        mp.add_forced_key_binding("ESC","pc-crop-escape",function() drag=nil;action="cancel";publish(true) end)
        mp.add_forced_key_binding("SPACE","pc-crop-space",function() end)
        ready=true
        redraw()
        publish(true)
    else
        overlay:remove()
        show_result()
    end
end)

mp.add_periodic_timer(0.033,function()
    if editing and ready then
        if drag then move_drag() else redraw() end
    end
end)

mp.set_property_native("user-data/pixelcut/crop-ready",true)
